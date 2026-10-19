package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Organization merge (open discrepancy X18). The survivor keeps its own fields and empty ones are
// filled from the duplicates. Everything attached to a duplicate moves to the survivor: memberships
// (every row stays except an exact duplicate of a survivor row), parent links (self links dropped,
// duplicates dropped, a cycle rejects the merge), deal links, task and interaction links and tags.
// Each duplicate is archived and a merge_alias row records that its id and uuid now resolve to the
// survivor (the same record that serves people, see ResolveAlias). Every row change is kept in the
// result so one undo step and one redo step can replay it exactly.

// OrgMergeEdit is one row change of a merge: the columns of a row and their value before and after.
type OrgMergeEdit struct {
	Table string
	ID    int64
	Cols  []string
	Old   []any
	New   []any
}

// OrgFieldChange says where a survivor field came from.
type OrgFieldChange struct {
	Field string `json:"field"`
	Value string `json:"value"`
	From  int64  `json:"from"`
}

// OrgMergeResult is what a merge did, or with DryRun what it would do.
type OrgMergeResult struct {
	KeepID    int64            `json:"keep_id"`
	KeepName  string           `json:"keep_name"`
	DropIDs   []int64          `json:"drop_ids"`
	DropNames []string         `json:"drop_names"`
	DryRun    bool             `json:"dry_run"`
	Fields    []OrgFieldChange `json:"fields"`
	// Lost lists fields where the duplicate had a different value than the survivor, which stays.
	Lost  []OrgFieldChange `json:"lost,omitempty"`
	Moved map[string]int   `json:"moved"`
	Edits []OrgMergeEdit   `json:"-"`
}

// Label is a short description for messages and the undo history.
func (r OrgMergeResult) Label() string {
	return fmt.Sprintf("merge %s into %s", strings.Join(r.DropNames, ", "), r.KeepName)
}

type orgMerger struct {
	ctx  context.Context
	tx   *sql.Tx
	keep int64
	res  *OrgMergeResult
}

func (m *orgMerger) edit(table string, id int64, cols []string, news ...any) error {
	old := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range old {
		ptrs[i] = &old[i]
	}
	if err := m.tx.QueryRowContext(m.ctx, fmt.Sprintf("SELECT %s FROM %s WHERE id = ?", strings.Join(cols, ", "), table), id).Scan(ptrs...); err != nil {
		return fmt.Errorf("read %s %d: %w", table, id, err)
	}
	e := OrgMergeEdit{Table: table, ID: id, Cols: cols, Old: old, New: news}
	if err := applyOrgEdit(m.ctx, m.tx, e, true); err != nil {
		return err
	}
	m.res.Edits = append(m.res.Edits, e)
	return nil
}

func (m *orgMerger) drop(table string, id int64) error {
	if err := m.edit(table, id, []string{"deleted_at"}, sql.NullString{String: "deleted", Valid: true}); err != nil {
		return err
	}
	m.res.Moved["duplicates_dropped"]++
	return nil
}

// applyOrgEdit writes the new values (forward) or the old ones (back). A soft delete stores a real
// timestamp, the "deleted" marker above is swapped for it here.
func applyOrgEdit(ctx context.Context, q DBTX, e OrgMergeEdit, forward bool) error {
	vals := e.Old
	if forward {
		vals = e.New
	}
	sets := make([]string, len(e.Cols))
	args := make([]any, 0, len(e.Cols)+1)
	for i, c := range e.Cols {
		v := vals[i]
		if ns, ok := v.(sql.NullString); ok && ns.Valid && ns.String == "deleted" && c == "deleted_at" {
			sets[i] = c + " = datetime('now')"
			continue
		}
		sets[i] = c + " = ?"
		args = append(args, v)
	}
	if e.Table != "taggings" {
		sets = append(sets, "updated_at = datetime('now')")
	}
	args = append(args, e.ID)
	_, err := q.ExecContext(ctx, "UPDATE "+e.Table+" SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	if err != nil {
		return fmt.Errorf("update %s %d: %w", e.Table, e.ID, err)
	}
	return nil
}

// MergeOrgs folds the duplicate organizations into the survivor in one transaction.
func (r *OrgRepo) MergeOrgs(ctx context.Context, keepID int64, dropIDs []int64) (*OrgMergeResult, error) {
	return r.mergeOrgs(ctx, keepID, dropIDs, false)
}

// MergeOrgsDryRun reports what MergeOrgs would do and writes nothing.
func (r *OrgRepo) MergeOrgsDryRun(ctx context.Context, keepID int64, dropIDs []int64) (*OrgMergeResult, error) {
	return r.mergeOrgs(ctx, keepID, dropIDs, true)
}

func (r *OrgRepo) mergeOrgs(ctx context.Context, keepID int64, dropIDs []int64, dryRun bool) (*OrgMergeResult, error) {
	if len(dropIDs) == 0 {
		return nil, fmt.Errorf("merge needs at least one organization to drop: %w", model.ErrValidation)
	}
	seen := map[int64]bool{keepID: true}
	for _, d := range dropIDs {
		if seen[d] {
			return nil, fmt.Errorf("organization %d is listed twice or is also the survivor: %w", d, model.ErrValidation)
		}
		seen[d] = true
	}
	keep, err := r.FindByID(ctx, keepID)
	if err != nil {
		return nil, err
	}
	res := &OrgMergeResult{KeepID: keepID, KeepName: keep.Name, DropIDs: dropIDs, DryRun: dryRun, Fields: []OrgFieldChange{},
		Moved: map[string]int{"memberships": 0, "relations": 0, "deals": 0, "tasks": 0, "interactions": 0, "tags": 0, "aliases": 0, "duplicates_dropped": 0}}
	drops := make([]*model.Organization, 0, len(dropIDs))
	for _, d := range dropIDs {
		o, err := r.FindByID(ctx, d)
		if err != nil {
			return nil, err
		}
		drops = append(drops, o)
		res.DropNames = append(res.DropNames, o.Name)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin organization merge: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	m := &orgMerger{ctx: ctx, tx: tx, keep: keepID, res: res}
	if err := m.fields(keep, drops); err != nil {
		return nil, err
	}
	for _, d := range drops {
		for _, step := range []func(int64) error{m.memberships, m.relations, m.dealLinks, m.activities, m.tags, m.alias} {
			if err := step(d.ID); err != nil {
				return nil, err
			}
		}
		if err := m.edit("organizations", d.ID, []string{"archived"}, 1); err != nil {
			return nil, err
		}
	}
	if err := m.checkCycle(); err != nil {
		return nil, err
	}
	if dryRun {
		return res, nil
	}
	if err := touchOrgMembers(ctx, tx, append([]int64{keepID}, dropIDs...)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit organization merge: %w", err)
	}
	return res, nil
}

// touchOrgMembers bumps updated_at of the people at the organizations, so sync and DAV see the new ORG.
func touchOrgMembers(ctx context.Context, q DBTX, ids []int64) error {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	_, err := q.ExecContext(ctx, `UPDATE people SET updated_at = datetime('now') WHERE id IN
		(SELECT person_id FROM membership WHERE org_id IN (`+placeholders(len(ids))+`))`, args...)
	if err != nil {
		return fmt.Errorf("touch members: %w", err)
	}
	return nil
}

// fields fills the survivor's empty fields from the duplicates, first duplicate with a value wins.
func (m *orgMerger) fields(keep *model.Organization, drops []*model.Organization) error {
	type fld struct {
		name string
		val  func(*model.Organization) *string
	}
	for _, f := range []fld{
		{"domain", func(o *model.Organization) *string { return o.Domain }},
		{"industry", func(o *model.Organization) *string { return o.Industry }},
		{"notes", func(o *model.Organization) *string { return o.Notes }},
		{"summary", func(o *model.Organization) *string { return o.Summary }},
		{"website", func(o *model.Organization) *string { return o.Website }},
		{"size", func(o *model.Organization) *string { return o.Size }},
		{"city", func(o *model.Organization) *string { return o.City }},
		{"region", func(o *model.Organization) *string { return o.Region }},
		{"country", func(o *model.Organization) *string { return o.Country }},
		{"linkedin_url", func(o *model.Organization) *string { return o.LinkedinURL }},
		{"description", func(o *model.Organization) *string { return o.Description }},
		{"logo_path", func(o *model.Organization) *string { return o.LogoPath }},
	} {
		empty := func(s *string) bool { return s == nil || strings.TrimSpace(*s) == "" }
		have := !empty(f.val(keep))
		filled := false
		for _, d := range drops {
			v := f.val(d)
			if empty(v) {
				continue
			}
			if !have && !filled {
				if err := m.edit("organizations", m.keep, []string{f.name}, *v); err != nil {
					return err
				}
				m.res.Fields = append(m.res.Fields, OrgFieldChange{Field: f.name, Value: *v, From: d.ID})
				filled = true
			} else if (have && *v != *f.val(keep)) || (filled && *v != m.res.Fields[len(m.res.Fields)-1].Value) {
				m.res.Lost = append(m.res.Lost, OrgFieldChange{Field: f.name, Value: *v, From: d.ID})
			}
		}
	}
	return nil
}

type orgMembership struct {
	id, person int64
	role, from, to sql.NullString
	primary        bool
}

func (m *orgMerger) orgMemberships(org int64) ([]orgMembership, error) {
	rows, err := m.tx.QueryContext(m.ctx, `SELECT id, person_id, role, started_on, ended_on, is_primary FROM membership
		WHERE org_id = ? AND deleted_at IS NULL ORDER BY id`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []orgMembership
	for rows.Next() {
		var r orgMembership
		var p int
		if err := rows.Scan(&r.id, &r.person, &r.role, &r.from, &r.to, &p); err != nil {
			return nil, err
		}
		r.primary = p == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

// memberships moves every membership of the duplicate. Only a row identical to one the survivor
// already has (same person, role, start and end) is dropped, and its primary flag moves to that row.
func (m *orgMerger) memberships(d int64) error {
	rows, err := m.orgMemberships(d)
	if err != nil {
		return fmt.Errorf("list memberships: %w", err)
	}
	same := func(a, b sql.NullString) bool { return a.Valid == b.Valid && a.String == b.String }
	for _, r := range rows {
		at, err := m.orgMemberships(m.keep)
		if err != nil {
			return err
		}
		var twin *orgMembership
		for i := range at {
			if at[i].person == r.person && same(at[i].role, r.role) && same(at[i].from, r.from) && same(at[i].to, r.to) {
				twin = &at[i]
				break
			}
		}
		if twin == nil {
			if err := m.edit("membership", r.id, []string{"org_id"}, m.keep); err != nil {
				return fmt.Errorf("move membership: %w", err)
			}
			m.res.Moved["memberships"]++
			continue
		}
		if err := m.drop("membership", r.id); err != nil {
			return err
		}
		if r.primary && !twin.primary {
			var n int
			if err := m.tx.QueryRowContext(m.ctx, `SELECT COUNT(*) FROM membership WHERE person_id = ? AND is_primary = 1 AND deleted_at IS NULL`, r.person).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				if err := m.edit("membership", twin.id, []string{"is_primary"}, 1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// relations moves parent and other organization links.
func (m *orgMerger) relations(d int64) error {
	rows, err := m.tx.QueryContext(m.ctx, `SELECT l.id, l.a_id, l.b_id, l.kind_id, k.symmetric FROM org_relation l
		JOIN relation_kind k ON k.id = l.kind_id WHERE (l.a_id = ? OR l.b_id = ?) AND l.deleted_at IS NULL ORDER BY l.id`, d, d)
	if err != nil {
		return fmt.Errorf("list organization links: %w", err)
	}
	type row struct {
		id, a, b, kind int64
		sym            bool
	}
	var found []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.a, &r.b, &r.kind, &r.sym); err != nil {
			rows.Close()
			return err
		}
		found = append(found, r)
	}
	rows.Close()
	for _, r := range found {
		a, b := r.a, r.b
		if a == d {
			a = m.keep
		}
		if b == d {
			b = m.keep
		}
		if a == b {
			if err := m.drop("org_relation", r.id); err != nil {
				return err
			}
			continue
		}
		if r.sym && a > b {
			a, b = b, a
		}
		var n int
		if err := m.tx.QueryRowContext(m.ctx, `SELECT COUNT(*) FROM org_relation WHERE a_id = ? AND b_id = ? AND kind_id = ?
			AND deleted_at IS NULL AND id <> ?`, a, b, r.kind, r.id).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			if err := m.drop("org_relation", r.id); err != nil {
				return err
			}
			continue
		}
		if err := m.edit("org_relation", r.id, []string{"a_id", "b_id"}, a, b); err != nil {
			return err
		}
		m.res.Moved["relations"]++
	}
	return nil
}

// checkCycle rejects a merge that leaves the survivor above itself in the parent hierarchy.
func (m *orgMerger) checkCycle() error {
	var n int
	err := m.tx.QueryRowContext(m.ctx, `WITH RECURSIVE anc(id) AS (
			SELECT l.a_id FROM org_relation l JOIN relation_kind k ON k.id = l.kind_id
			 WHERE l.b_id = ?1 AND k.builtin = 1 AND k.object_type = 'organization' AND k.symmetric = 0 AND l.deleted_at IS NULL
			UNION
			SELECT l.a_id FROM org_relation l JOIN anc ON l.b_id = anc.id JOIN relation_kind k ON k.id = l.kind_id
			 WHERE k.builtin = 1 AND k.object_type = 'organization' AND k.symmetric = 0 AND l.deleted_at IS NULL)
		SELECT COUNT(*) FROM anc WHERE id = ?1`, m.keep).Scan(&n)
	if err != nil {
		return fmt.Errorf("check cycle: %w", err)
	}
	if n > 0 {
		return fmt.Errorf("merging would make organization %d its own parent through the parent links, remove one of the parent links first: %w", m.keep, model.ErrValidation)
	}
	return nil
}

// dealLinks moves deal links. A deal that already has the survivor keeps its row, and the primary
// flag moves to it when the duplicate's row was primary.
func (m *orgMerger) dealLinks(d int64) error {
	rows, err := m.tx.QueryContext(m.ctx, `SELECT id, deal_id, is_primary FROM deal_org WHERE org_id = ? AND deleted_at IS NULL ORDER BY id`, d)
	if err != nil {
		return fmt.Errorf("list deal links: %w", err)
	}
	type row struct {
		id, deal int64
		primary  bool
	}
	var found []row
	for rows.Next() {
		var r row
		var p int
		if err := rows.Scan(&r.id, &r.deal, &p); err != nil {
			rows.Close()
			return err
		}
		r.primary = p == 1
		found = append(found, r)
	}
	rows.Close()
	for _, r := range found {
		var keepRow int64
		var keepPrimary int
		err := m.tx.QueryRowContext(m.ctx, `SELECT id, is_primary FROM deal_org WHERE deal_id = ? AND org_id = ? AND deleted_at IS NULL`, r.deal, m.keep).Scan(&keepRow, &keepPrimary)
		switch err {
		case nil:
			if err := m.drop("deal_org", r.id); err != nil {
				return err
			}
			if r.primary && keepPrimary == 0 {
				if err := m.edit("deal_org", keepRow, []string{"is_primary"}, 1); err != nil {
					return err
				}
			}
		case sql.ErrNoRows:
			if err := m.edit("deal_org", r.id, []string{"org_id"}, m.keep); err != nil {
				return err
			}
			m.res.Moved["deals"]++
		default:
			return err
		}
	}
	return nil
}

// activities moves task and interaction links.
func (m *orgMerger) activities(d int64) error {
	rows, err := m.tx.QueryContext(m.ctx, `SELECT id, source_kind, source_id FROM activity_target
		WHERE target_kind = 'organization' AND target_id = ? AND deleted_at IS NULL ORDER BY id`, d)
	if err != nil {
		return fmt.Errorf("list activity links: %w", err)
	}
	type row struct {
		id, src int64
		kind    string
	}
	var found []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.kind, &r.src); err != nil {
			rows.Close()
			return err
		}
		found = append(found, r)
	}
	rows.Close()
	for _, r := range found {
		var n int
		if err := m.tx.QueryRowContext(m.ctx, `SELECT COUNT(*) FROM activity_target WHERE source_kind = ? AND source_id = ?
			AND target_kind = 'organization' AND target_id = ? AND deleted_at IS NULL`, r.kind, r.src, m.keep).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			if err := m.drop("activity_target", r.id); err != nil {
				return err
			}
			continue
		}
		if err := m.edit("activity_target", r.id, []string{"target_id"}, m.keep); err != nil {
			return err
		}
		if r.kind == "interaction" {
			m.res.Moved["interactions"]++
		} else {
			m.res.Moved["tasks"]++
		}
	}
	return nil
}

// tags moves the duplicate's tags the survivor does not have. A tag both have stays on the archived duplicate.
func (m *orgMerger) tags(d int64) error {
	rows, err := m.tx.QueryContext(m.ctx, `SELECT id FROM taggings WHERE entity_type = 'organization' AND entity_id = ?
		AND tag_id NOT IN (SELECT tag_id FROM taggings WHERE entity_type = 'organization' AND entity_id = ?) ORDER BY id`, d, m.keep)
	if err != nil {
		return fmt.Errorf("list tags: %w", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if _, err := m.tx.ExecContext(m.ctx, "UPDATE taggings SET entity_id = ? WHERE id = ?", m.keep, id); err != nil {
			return fmt.Errorf("move tag: %w", err)
		}
		m.res.Edits = append(m.res.Edits, OrgMergeEdit{Table: "taggings", ID: id, Cols: []string{"entity_id"}, Old: []any{d}, New: []any{m.keep}})
		m.res.Moved["tags"]++
	}
	return nil
}

// alias records that the duplicate's id and uuid now resolve to the survivor, and repoints aliases
// that already pointed at the duplicate.
func (m *orgMerger) alias(d int64) error {
	rows, err := m.tx.QueryContext(m.ctx, `SELECT id FROM merge_alias WHERE kind = 'organization' AND survivor_id = ? AND deleted_at IS NULL`, d)
	if err != nil {
		return fmt.Errorf("list aliases: %w", err)
	}
	var older []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		older = append(older, id)
	}
	rows.Close()
	for _, id := range older {
		if err := m.edit("merge_alias", id, []string{"survivor_id"}, m.keep); err != nil {
			return err
		}
	}
	var oldUUID string
	if err := m.tx.QueryRowContext(m.ctx, "SELECT uuid FROM organizations WHERE id = ?", d).Scan(&oldUUID); err != nil {
		return err
	}
	var existing int64
	err = m.tx.QueryRowContext(m.ctx, `SELECT id FROM merge_alias WHERE kind = 'organization' AND old_id = ? AND deleted_at IS NULL`, d).Scan(&existing)
	switch err {
	case nil:
		if err := m.edit("merge_alias", existing, []string{"survivor_id"}, m.keep); err != nil {
			return err
		}
	case sql.ErrNoRows:
		res, err := m.tx.ExecContext(m.ctx, `INSERT INTO merge_alias (uuid, kind, old_id, old_uuid, survivor_id) VALUES (?, 'organization', ?, ?, ?)`,
			uuid.New().String(), d, oldUUID, m.keep)
		if err != nil {
			return fmt.Errorf("write merge alias: %w", err)
		}
		id, _ := res.LastInsertId()
		// Undo hides the row again, redo brings it back.
		m.res.Edits = append(m.res.Edits, OrgMergeEdit{Table: "merge_alias", ID: id, Cols: []string{"deleted_at"},
			Old: []any{sql.NullString{String: "deleted", Valid: true}}, New: []any{nil}})
	default:
		return err
	}
	m.res.Moved["aliases"]++
	return nil
}

// UndoMerge puts back what a merge changed, in reverse order. It touches the members again so sync sees it.
func (r *OrgRepo) UndoMerge(ctx context.Context, res *OrgMergeResult) error {
	return r.replayMerge(ctx, res, false)
}

// RedoMerge repeats a merge that UndoMerge reversed.
func (r *OrgRepo) RedoMerge(ctx context.Context, res *OrgMergeResult) error {
	return r.replayMerge(ctx, res, true)
}

func (r *OrgRepo) replayMerge(ctx context.Context, res *OrgMergeResult, forward bool) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replay: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	n := len(res.Edits)
	for i := 0; i < n; i++ {
		e := res.Edits[i]
		if !forward {
			e = res.Edits[n-1-i]
		}
		if err := applyOrgEdit(ctx, tx, e, forward); err != nil {
			return err
		}
	}
	if err := touchOrgMembers(ctx, tx, append([]int64{res.KeepID}, res.DropIDs...)); err != nil {
		return err
	}
	return tx.Commit()
}
