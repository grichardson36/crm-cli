package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Ref names one object: its kind (person, organization, deal, task, interaction) and id.
type Ref struct {
	Kind string
	ID   int64
}

func (r Ref) String() string { return fmt.Sprintf("%s %d", r.Kind, r.ID) }

// LinkResult says what Link did. Undo uses it: a created link is soft-deleted, an updated link gets
// Before written back, and Demoted links get their primary flag back.
type LinkResult struct {
	Changed  bool
	Created  bool
	Updated  bool
	Relation string
	LinkID   int64
	// Before is the link as it was before an update.
	Before *LinkRecord
	// Demoted are the ids of links in the same relation that lost their primary flag.
	Demoted []int64
}

// LinkRecord is one stored link row in full. It is what undo needs to put a link back exactly.
type LinkRecord struct {
	Relation  string  `json:"relation"`
	ID        int64   `json:"id"`
	A         int64   `json:"a"`
	B         int64   `json:"b"`
	Role      *string `json:"role,omitempty"`
	StartedOn *string `json:"from,omitempty"`
	EndedOn   *string `json:"to,omitempty"`
	Primary   bool    `json:"primary,omitempty"`
	KindID    *int64  `json:"kind_id,omitempty"`
	Notes     *string `json:"notes,omitempty"`
	Deleted   bool    `json:"deleted,omitempty"`
}

// LinkRepo is the one place that makes, lists, edits and removes links between objects
// (model.Relations). Every door calls it with the same names.
type LinkRepo struct {
	db    *sql.DB
	kinds *KindRepo
	now   func() time.Time
}

// NewLinkRepo creates a LinkRepo.
func NewLinkRepo(db *sql.DB) *LinkRepo {
	return &LinkRepo{db: db, kinds: NewKindRepo(db), now: time.Now}
}

// DB returns the database the repository works on.
func (r *LinkRepo) DB() *sql.DB { return r.db }

// tableOf maps an object kind to its table. Names come from this fixed map, never from input.
var tableOf = map[string]string{"person": "people", "organization": "organizations", "deal": "deals", "task": "tasks", "interaction": "interactions"}

// nameExpr is the SQL that names an object of a kind, for the alias o.
var nameExpr = map[string]string{
	"person":       "trim(o.first_name || ' ' || COALESCE(o.last_name, ''))",
	"organization": "o.name",
	"deal":         "o.title",
	"task":         "o.title",
	"interaction":  "COALESCE(o.subject, o.type)",
}

// store maps a relation to its table and columns.
type store struct {
	table, aCol, bCol string
	fixed             map[string]string // activity_target kind columns
	cols              map[string]string // attribute key to column
}

func storeOf(rel model.Relation) store {
	switch rel.Name {
	case "membership":
		return store{table: "membership", aCol: "person_id", bCol: "org_id", cols: map[string]string{
			model.AttrRole: "role", model.AttrFrom: "started_on", model.AttrTo: "ended_on", model.AttrPrimary: "is_primary", model.AttrNotes: "notes"}}
	case "person_relation":
		return store{table: "person_relation", aCol: "a_id", bCol: "b_id", cols: map[string]string{model.AttrKind: "kind_id", model.AttrNotes: "notes"}}
	case "org_relation":
		return store{table: "org_relation", aCol: "a_id", bCol: "b_id", cols: map[string]string{model.AttrKind: "kind_id", model.AttrNotes: "notes"}}
	case "deal_person":
		return store{table: "deal_person", aCol: "deal_id", bCol: "person_id", cols: map[string]string{model.AttrRole: "role"}}
	case "deal_org":
		return store{table: "deal_org", aCol: "deal_id", bCol: "org_id", cols: map[string]string{model.AttrRole: "role", model.AttrPrimary: "is_primary"}}
	}
	return store{table: "activity_target", aCol: "source_id", bCol: "target_id",
		fixed: map[string]string{"source_kind": rel.A, "target_kind": rel.B}, cols: map[string]string{model.AttrRole: "role"}}
}

func (s store) col(key string) string {
	if c, ok := s.cols[key]; ok {
		return c
	}
	return "NULL"
}

// selectList is the column list that fills a linkRow, with NULL for columns the table lacks.
func (s store) selectList(prefix string) string {
	p := func(c string) string {
		if c == "NULL" {
			return c
		}
		return prefix + c
	}
	primary := p(s.col(model.AttrPrimary))
	if primary == "NULL" {
		primary = "0"
	}
	return fmt.Sprintf("%sid, %s%s, %s%s, %s, %s, %s, %s, %s, %s, %sdeleted_at IS NOT NULL",
		prefix, prefix, s.aCol, prefix, s.bCol, p(s.col(model.AttrRole)), p(s.col(model.AttrFrom)), p(s.col(model.AttrTo)),
		primary, p(s.col(model.AttrKind)), p(s.col(model.AttrNotes)), prefix)
}

// fixedWhere returns " AND col = 'value'" for the activity kind columns.
func (s store) fixedWhere(prefix string) string {
	var cols []string
	for c := range s.fixed {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	var b strings.Builder
	for _, c := range cols {
		fmt.Fprintf(&b, " AND %s%s = '%s'", prefix, c, s.fixed[c]) // values are object kind names from the registry
	}
	return b.String()
}

func scanRecord(rel model.Relation, row interface{ Scan(...any) error }) (*LinkRecord, error) {
	var rec LinkRecord
	var primary int
	err := row.Scan(&rec.ID, &rec.A, &rec.B, &rec.Role, &rec.StartedOn, &rec.EndedOn, &primary, &rec.KindID, &rec.Notes, &rec.Deleted)
	if err != nil {
		return nil, err
	}
	rec.Relation = rel.Name
	rec.Primary = primary == 1
	return &rec, nil
}

func (r *LinkRepo) check(ctx context.Context, x Ref) error {
	table, ok := tableOf[x.Kind]
	if !ok {
		return fmt.Errorf("unknown object %q (use one of: person, organization, deal, task): %w", x.Kind, model.ErrValidation)
	}
	var archived int
	err := r.db.QueryRowContext(ctx, "SELECT archived FROM "+table+" WHERE id = ?", x.ID).Scan(&archived)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && archived != 0) {
		return fmt.Errorf("%s %d: %w", x.Kind, x.ID, model.ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("find %s %d: %w", x.Kind, x.ID, err)
	}
	return nil
}

// orient returns the relation and the two refs in the relation's A, B order. For a relation
// between two objects of one kind the order given is kept.
func (r *LinkRepo) orient(ctx context.Context, a, b Ref) (model.Relation, Ref, Ref, error) {
	if err := r.check(ctx, a); err != nil {
		return model.Relation{}, a, b, err
	}
	if err := r.check(ctx, b); err != nil {
		return model.Relation{}, a, b, err
	}
	rel, ok := model.RelationBetween(a.Kind, b.Kind)
	if !ok {
		return rel, a, b, fmt.Errorf("a %s cannot link to a %s: %w", a.Kind, b.Kind, model.ErrValidation)
	}
	if rel.SameKind() && a.ID == b.ID {
		return rel, a, b, fmt.Errorf("%s cannot be linked to itself: %w", a, model.ErrValidation)
	}
	if rel.A == a.Kind {
		return rel, a, b, nil
	}
	return rel, b, a, nil
}

// validateAttrs rejects attributes the relation does not carry and bad dates.
func (r *LinkRepo) validateAttrs(rel model.Relation, at model.LinkAttrs) (model.LinkAttrs, error) {
	has := map[string]bool{
		model.AttrRole: at.Role != nil, model.AttrFrom: at.StartedOn != nil, model.AttrTo: at.EndedOn != nil,
		model.AttrPrimary: at.Primary != nil, model.AttrKind: at.Kind != nil, model.AttrNotes: at.Notes != nil,
	}
	for key, given := range has {
		if given && !rel.HasAttr(key) {
			return at, fmt.Errorf("a %s-%s link has no %q (it carries: %s): %w", rel.A, rel.B, key, strings.Join(rel.Attrs, ", "), model.ErrValidation)
		}
	}
	now := r.now()
	for _, p := range []**string{&at.StartedOn, &at.EndedOn} {
		if *p != nil {
			d, err := model.NormalizeLinkDate(**p, now)
			if err != nil {
				return at, err
			}
			*p = &d
		}
	}
	if at.Role != nil {
		v := strings.TrimSpace(*at.Role)
		at.Role = &v
	}
	return at, nil
}

func nilIfEmptyPtr(p *string) any {
	if p == nil || *p == "" {
		return nil
	}
	return *p
}

// LinkNew is Link that always adds another link, even when the pair is already linked. Only
// memberships can repeat (a person who left and came back, or holds two roles).
func (r *LinkRepo) LinkNew(ctx context.Context, a, b Ref, attrs ...model.LinkAttrs) (LinkResult, error) {
	return r.link(ctx, a, b, firstAttrs(attrs), true)
}

// Link connects a and b with the given attributes. Linking what is already linked with the same
// attributes changes nothing. When one live link joins the pair, attributes that are given change
// on it. When several do, the call fails and asks for LinkNew or Update by id.
func (r *LinkRepo) Link(ctx context.Context, a, b Ref, attrs ...model.LinkAttrs) (LinkResult, error) {
	return r.link(ctx, a, b, firstAttrs(attrs), false)
}

func firstAttrs(a []model.LinkAttrs) model.LinkAttrs {
	if len(a) == 0 {
		return model.LinkAttrs{}
	}
	return a[0]
}

func (r *LinkRepo) link(ctx context.Context, a, b Ref, attrs model.LinkAttrs, forceNew bool) (LinkResult, error) {
	rel, ra, rb, err := r.orient(ctx, a, b)
	if err != nil {
		return LinkResult{}, err
	}
	if forceNew && rel.Unique {
		return LinkResult{}, fmt.Errorf("%s links are unique per pair: %w", rel.Name, model.ErrValidation)
	}
	attrs, err = r.validateAttrs(rel, attrs)
	if err != nil {
		return LinkResult{}, err
	}
	st := storeOf(rel)

	var kind *model.RelationKind
	if rel.Kinded {
		name := ""
		if attrs.Kind != nil {
			name = *attrs.Kind
		} else if rel.A == "person" {
			name = model.DefaultPersonKind
		} else {
			name = model.DefaultOrgKind
		}
		var swapped bool
		kind, swapped, err = r.kinds.Resolve(ctx, rel.A, name)
		if err != nil {
			return LinkResult{}, err
		}
		if swapped {
			ra, rb = rb, ra
		}
		if kind.Symmetric && ra.ID > rb.ID {
			ra, rb = rb, ra
		}
		if rel.Name == "org_relation" && kind.Builtin && kind.Name == model.DefaultOrgKind {
			if err := r.checkOrgCycle(ctx, kind.ID, ra.ID, rb.ID); err != nil {
				return LinkResult{}, err
			}
		}
	}

	existing, err := r.liveRows(ctx, rel, ra.ID, rb.ID)
	if err != nil {
		return LinkResult{}, err
	}
	if rel.Kinded {
		var same []*LinkRecord
		for _, e := range existing {
			if e.KindID != nil && *e.KindID == kind.ID {
				same = append(same, e)
			}
		}
		existing = same
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return LinkResult{}, fmt.Errorf("begin link: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res := LinkResult{Relation: rel.Name}
	switch {
	case len(existing) == 0 || forceNew:
		id, demoted, err := r.insert(ctx, tx, rel, st, ra.ID, rb.ID, attrs, kind)
		if err != nil {
			return LinkResult{}, err
		}
		res.Changed, res.Created, res.LinkID, res.Demoted = true, true, id, demoted
	case len(existing) > 1:
		return LinkResult{}, fmt.Errorf("%s and %s are linked %d times, so say which link to change (link id) or add another: %w", ra, rb, len(existing), model.ErrConflict)
	default:
		cur := existing[0]
		res.LinkID = cur.ID
		next, changed := applyAttrs(*cur, attrs, kind)
		if !changed {
			return res, nil
		}
		if next.StartedOn != nil && next.EndedOn != nil && *next.StartedOn > *next.EndedOn {
			return LinkResult{}, fmt.Errorf("the end date is before the start date: %w", model.ErrValidation)
		}
		before := *cur
		res.Before = &before
		demoted, err := r.write(ctx, tx, rel, st, next)
		if err != nil {
			return LinkResult{}, err
		}
		res.Changed, res.Updated, res.Demoted = true, true, demoted
	}
	if err := r.touchBoth(ctx, tx, rel, ra.ID, rb.ID); err != nil {
		return LinkResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return LinkResult{}, fmt.Errorf("commit link: %w", err)
	}
	return res, nil
}

// applyAttrs applies given attributes to a record and says whether anything changed.
func applyAttrs(cur LinkRecord, at model.LinkAttrs, kind *model.RelationKind) (LinkRecord, bool) {
	changed := false
	set := func(dst **string, v *string) {
		if v == nil {
			return
		}
		var nv *string
		if *v != "" {
			s := *v
			nv = &s
		}
		if (*dst == nil) != (nv == nil) || (nv != nil && **dst != *nv) {
			changed = true
		}
		*dst = nv
	}
	set(&cur.Role, at.Role)
	set(&cur.StartedOn, at.StartedOn)
	set(&cur.EndedOn, at.EndedOn)
	set(&cur.Notes, at.Notes)
	if at.Primary != nil && *at.Primary != cur.Primary {
		cur.Primary, changed = *at.Primary, true
	}
	_ = kind
	return cur, changed
}

// liveRows lists the live links between a pair, in relation order.
func (r *LinkRepo) liveRows(ctx context.Context, rel model.Relation, aID, bID int64) ([]*LinkRecord, error) {
	st := storeOf(rel)
	q := fmt.Sprintf("SELECT %s FROM %s WHERE %s = ? AND %s = ? AND deleted_at IS NULL%s ORDER BY id",
		st.selectList(""), st.table, st.aCol, st.bCol, st.fixedWhere(""))
	rows, err := r.db.QueryContext(ctx, q, aID, bID)
	if err != nil {
		return nil, fmt.Errorf("find links: %w", err)
	}
	defer rows.Close()
	var out []*LinkRecord
	for rows.Next() {
		rec, err := scanRecord(rel, rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// insert adds a link row, handling the primary flag. It returns the new id and the demoted ids.
func (r *LinkRepo) insert(ctx context.Context, tx *sql.Tx, rel model.Relation, st store, aID, bID int64, at model.LinkAttrs, kind *model.RelationKind) (int64, []int64, error) {
	cols := []string{"uuid", st.aCol, st.bCol}
	vals := []any{uuid.New().String(), aID, bID}
	for c := range st.fixed {
		cols = append(cols, c)
		vals = append(vals, st.fixed[c])
	}
	add := func(key string, v any) {
		if c, ok := st.cols[key]; ok {
			cols = append(cols, c)
			vals = append(vals, v)
		}
	}
	add(model.AttrRole, nilIfEmptyPtr(at.Role))
	add(model.AttrFrom, nilIfEmptyPtr(at.StartedOn))
	add(model.AttrTo, nilIfEmptyPtr(at.EndedOn))
	add(model.AttrNotes, nilIfEmptyPtr(at.Notes))
	if kind != nil {
		add(model.AttrKind, kind.ID)
	}
	var demoted []int64
	if rel.PrimaryPerA {
		want := false
		if at.Primary != nil {
			want = *at.Primary
		} else if at.EndedOn == nil || *at.EndedOn == "" {
			has, err := r.hasVisiblePrimary(ctx, tx, rel, st, aID)
			if err != nil {
				return 0, nil, err
			}
			want = !has
		}
		if want {
			d, err := r.demoteOthers(ctx, tx, st, aID, 0)
			if err != nil {
				return 0, nil, err
			}
			demoted = d
			add(model.AttrPrimary, 1)
		} else {
			add(model.AttrPrimary, 0)
		}
	}
	ph := strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", ")
	res, err := tx.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", st.table, strings.Join(cols, ", "), ph), vals...)
	if err != nil {
		return 0, nil, fmt.Errorf("link: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, demoted, nil
}

// hasVisiblePrimary reports whether the A object has a live primary link whose B object is live.
func (r *LinkRepo) hasVisiblePrimary(ctx context.Context, tx *sql.Tx, rel model.Relation, st store, aID int64) (bool, error) {
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s l JOIN %s o ON o.id = l.%s WHERE l.%s = ? AND l.is_primary = 1 AND l.deleted_at IS NULL AND o.archived = 0",
		st.table, tableOf[rel.B], st.bCol, st.aCol)
	var n int
	if err := tx.QueryRowContext(ctx, q, aID).Scan(&n); err != nil {
		return false, fmt.Errorf("find primary: %w", err)
	}
	return n > 0, nil
}

// demoteOthers clears the primary flag of the other live links of the A object.
func (r *LinkRepo) demoteOthers(ctx context.Context, tx *sql.Tx, st store, aID, exceptID int64) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("SELECT id FROM %s WHERE %s = ? AND is_primary = 1 AND deleted_at IS NULL AND id <> ?", st.table, st.aCol), aID, exceptID)
	if err != nil {
		return nil, fmt.Errorf("find primary: %w", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET is_primary = 0, updated_at = datetime('now') WHERE id = ?", st.table), id); err != nil {
			return nil, fmt.Errorf("demote primary: %w", err)
		}
	}
	return ids, nil
}

// write stores every attribute of rec on its row, and moves the primary flag when it is set.
func (r *LinkRepo) write(ctx context.Context, tx *sql.Tx, rel model.Relation, st store, rec LinkRecord) ([]int64, error) {
	var demoted []int64
	if rel.PrimaryPerA && rec.Primary && !rec.Deleted {
		d, err := r.demoteOthers(ctx, tx, st, rec.A, rec.ID)
		if err != nil {
			return nil, err
		}
		demoted = d
	}
	var sets []string
	var args []any
	add := func(key string, v any) {
		if c, ok := st.cols[key]; ok {
			sets = append(sets, c+" = ?")
			args = append(args, v)
		}
	}
	add(model.AttrRole, rec.Role)
	add(model.AttrFrom, rec.StartedOn)
	add(model.AttrTo, rec.EndedOn)
	add(model.AttrNotes, rec.Notes)
	add(model.AttrPrimary, rec.Primary)
	if rel.Kinded && rec.KindID != nil {
		add(model.AttrKind, *rec.KindID)
	}
	sets = append(sets, "updated_at = datetime('now')")
	args = append(args, rec.ID)
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s WHERE id = ?", st.table, strings.Join(sets, ", ")), args...); err != nil {
		return nil, fmt.Errorf("update link: %w", err)
	}
	return demoted, nil
}

// touchBoth bumps updated_at of the two objects so sync and DAV see the change. Activity ends
// (tasks and interactions) are touched too.
func (r *LinkRepo) touchBoth(ctx context.Context, tx *sql.Tx, rel model.Relation, aID, bID int64) error {
	for _, x := range []Ref{{rel.A, aID}, {rel.B, bID}} {
		if _, err := tx.ExecContext(ctx, "UPDATE "+tableOf[x.Kind]+" SET updated_at = datetime('now') WHERE id = ?", x.ID); err != nil {
			return fmt.Errorf("touch %s: %w", x, err)
		}
	}
	return nil
}

// Get returns one stored link, deleted or not.
func (r *LinkRepo) Get(ctx context.Context, relation string, id int64) (*LinkRecord, error) {
	rel, ok := model.RelationNamed(relation)
	if !ok {
		return nil, fmt.Errorf("unknown relation %q: %w", relation, model.ErrValidation)
	}
	st := storeOf(rel)
	rec, err := scanRecord(rel, r.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT %s FROM %s WHERE id = ?%s", st.selectList(""), st.table, st.fixedWhere("")), id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%s link %d: %w", relation, id, model.ErrNotFound)
	}
	return rec, err
}

// Update changes attributes of one link by id.
func (r *LinkRepo) Update(ctx context.Context, relation string, id int64, attrs model.LinkAttrs) (LinkResult, error) {
	rel, ok := model.RelationNamed(relation)
	if !ok {
		return LinkResult{}, fmt.Errorf("unknown relation %q: %w", relation, model.ErrValidation)
	}
	cur, err := r.Get(ctx, relation, id)
	if err != nil {
		return LinkResult{}, err
	}
	if cur.Deleted {
		return LinkResult{}, fmt.Errorf("%s link %d: %w", relation, id, model.ErrNotFound)
	}
	attrs, err = r.validateAttrs(rel, attrs)
	if err != nil {
		return LinkResult{}, err
	}
	var kind *model.RelationKind
	kindChanged := false
	next := *cur
	if attrs.Kind != nil {
		k, _, err := r.kinds.Resolve(ctx, rel.A, *attrs.Kind)
		if err != nil {
			return LinkResult{}, err
		}
		kind = k
		kindChanged = cur.KindID == nil || *cur.KindID != k.ID
		next.KindID = &k.ID
	}
	next, changed := applyAttrs(next, attrs, kind)
	changed = changed || kindChanged
	res := LinkResult{Relation: relation, LinkID: id}
	if !changed {
		return res, nil
	}
	if next.StartedOn != nil && next.EndedOn != nil && *next.StartedOn > *next.EndedOn {
		return LinkResult{}, fmt.Errorf("the end date is before the start date: %w", model.ErrValidation)
	}
	before, _ := r.Get(ctx, relation, id)
	res.Before = before
	return r.overwrite(ctx, rel, next, res)
}

// Overwrite writes a stored link exactly as rec says, including empty values. Undo and redo use it.
func (r *LinkRepo) Overwrite(ctx context.Context, rec LinkRecord) (LinkResult, error) {
	rel, ok := model.RelationNamed(rec.Relation)
	if !ok {
		return LinkResult{}, fmt.Errorf("unknown relation %q: %w", rec.Relation, model.ErrValidation)
	}
	before, err := r.Get(ctx, rec.Relation, rec.ID)
	if err != nil {
		return LinkResult{}, err
	}
	return r.overwrite(ctx, rel, rec, LinkResult{Relation: rec.Relation, LinkID: rec.ID, Before: before})
}

func (r *LinkRepo) overwrite(ctx context.Context, rel model.Relation, rec LinkRecord, res LinkResult) (LinkResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return LinkResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	st := storeOf(rel)
	demoted, err := r.write(ctx, tx, rel, st, rec)
	if err != nil {
		return LinkResult{}, err
	}
	if err := r.touchBoth(ctx, tx, rel, rec.A, rec.B); err != nil {
		return LinkResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return LinkResult{}, err
	}
	res.Changed, res.Updated, res.Demoted = true, true, demoted
	return res, nil
}

// UnlinkOptions narrows Unlink when a pair is linked more than once.
type UnlinkOptions struct {
	// LinkID removes only that link.
	LinkID int64
	// Kind removes only links of that relation kind (person and organization relations).
	Kind string
}

// Unlink soft-deletes the live links between a and b and returns them, so an undo can restore them.
// It is an error when they are not linked.
func (r *LinkRepo) Unlink(ctx context.Context, a, b Ref, opts ...UnlinkOptions) ([]*LinkRecord, error) {
	var o UnlinkOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	rel, ra, rb, err := r.orient(ctx, a, b)
	if err != nil {
		return nil, err
	}
	var recs []*LinkRecord
	pairs := [][2]int64{{ra.ID, rb.ID}}
	if rel.SameKind() {
		pairs = append(pairs, [2]int64{rb.ID, ra.ID})
	}
	for _, p := range pairs {
		found, err := r.liveRows(ctx, rel, p[0], p[1])
		if err != nil {
			return nil, err
		}
		recs = append(recs, found...)
	}
	if o.LinkID != 0 {
		var keep []*LinkRecord
		for _, x := range recs {
			if x.ID == o.LinkID {
				keep = append(keep, x)
			}
		}
		recs = keep
	}
	if o.Kind != "" {
		k, _, err := r.kinds.Resolve(ctx, rel.A, o.Kind)
		if err != nil {
			return nil, err
		}
		var keep []*LinkRecord
		for _, x := range recs {
			if x.KindID != nil && *x.KindID == k.ID {
				keep = append(keep, x)
			}
		}
		recs = keep
	}
	if len(recs) == 0 {
		return nil, fmt.Errorf("%s is not linked to %s: %w", a, b, model.ErrNotFound)
	}
	st := storeOf(rel)
	for _, x := range recs {
		if err := r.setDeleted(ctx, rel, st, x, true); err != nil {
			return nil, err
		}
	}
	return recs, nil
}

func (r *LinkRepo) setDeleted(ctx context.Context, rel model.Relation, st store, x *LinkRecord, deleted bool) error {
	val := "NULL"
	if deleted {
		val = "datetime('now')"
	}
	if _, err := r.db.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET deleted_at = %s, updated_at = datetime('now') WHERE id = ?", st.table, val), x.ID); err != nil {
		return fmt.Errorf("unlink: %w", err)
	}
	for _, o := range []Ref{{rel.A, x.A}, {rel.B, x.B}} {
		if _, err := r.db.ExecContext(ctx, "UPDATE "+tableOf[o.Kind]+" SET updated_at = datetime('now') WHERE id = ?", o.ID); err != nil {
			return fmt.Errorf("touch %s: %w", o, err)
		}
	}
	return nil
}

// RestoreLink brings back a link that Unlink removed. If it was the primary and another link has
// taken the flag since, the restored link comes back without it.
func (r *LinkRepo) RestoreLink(ctx context.Context, relation string, id int64) error {
	rel, ok := model.RelationNamed(relation)
	if !ok {
		return fmt.Errorf("unknown relation %q: %w", relation, model.ErrValidation)
	}
	rec, err := r.Get(ctx, relation, id)
	if err != nil {
		return err
	}
	st := storeOf(rel)
	if rec.Primary && rel.PrimaryPerA {
		var n int
		_ = r.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ? AND is_primary = 1 AND deleted_at IS NULL", st.table, st.aCol), rec.A).Scan(&n)
		if n > 0 {
			if _, err := r.db.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET is_primary = 0 WHERE id = ?", st.table), id); err != nil {
				return err
			}
		}
	}
	return r.setDeleted(ctx, rel, st, rec, false)
}

// checkOrgCycle rejects a parent link that would make an organization its own ancestor. The row
// (a, b, parent) makes a the parent of b, so it is a cycle when b is already an ancestor of a.
func (r *LinkRepo) checkOrgCycle(ctx context.Context, parentKindID, aID, bID int64) error {
	var n int
	err := r.db.QueryRowContext(ctx, `WITH RECURSIVE anc(id) AS (
			SELECT ? UNION
			SELECT l.a_id FROM org_relation l JOIN anc ON l.b_id = anc.id
			WHERE l.kind_id = ? AND l.deleted_at IS NULL)
		SELECT COUNT(*) FROM anc WHERE id = ?`, aID, parentKindID, bID).Scan(&n)
	if err != nil {
		return fmt.Errorf("check cycle: %w", err)
	}
	if n > 0 {
		return fmt.Errorf("organization %d is already above organization %d, so this parent link would make a cycle: %w", bID, aID, model.ErrValidation)
	}
	return nil
}

// Linked lists the links of a to objects of otherKind, live links to live objects only, with the
// link attributes. Current memberships come before past ones, the primary first.
func (r *LinkRepo) Linked(ctx context.Context, a Ref, otherKind string) ([]model.LinkedObject, error) {
	if err := r.check(ctx, a); err != nil {
		return nil, err
	}
	rel, ok := model.RelationBetween(a.Kind, otherKind)
	if !ok {
		return nil, fmt.Errorf("a %s cannot link to a %s: %w", a.Kind, otherKind, model.ErrValidation)
	}
	return r.linkedVia(ctx, rel, a)
}

// All lists every link of a across all its relations except notes: they are a timeline, `log list`
// shows them.
func (r *LinkRepo) All(ctx context.Context, a Ref) ([]model.LinkedObject, error) {
	if err := r.check(ctx, a); err != nil {
		return nil, err
	}
	var out []model.LinkedObject
	for _, rel := range model.RelationsOf(a.Kind) {
		if rel.A == "interaction" {
			continue
		}
		items, err := r.linkedVia(ctx, rel, a)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

// LinkedIDs is Linked reduced to the ids.
func (r *LinkRepo) LinkedIDs(ctx context.Context, a Ref, otherKind string) ([]int64, error) {
	items, err := r.Linked(ctx, a, otherKind)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	return ids, nil
}

func (r *LinkRepo) linkedVia(ctx context.Context, rel model.Relation, a Ref) ([]model.LinkedObject, error) {
	st := storeOf(rel)
	today := r.now().Format("2006-01-02")
	var out []model.LinkedObject
	sides := []bool{}
	if rel.A == a.Kind {
		sides = append(sides, true)
	}
	if rel.B == a.Kind {
		sides = append(sides, false)
	}
	for _, anchorIsA := range sides {
		anchorCol, otherCol, otherKind := st.aCol, st.bCol, rel.B
		if !anchorIsA {
			anchorCol, otherCol, otherKind = st.bCol, st.aCol, rel.A
		}
		label := "''"
		join := ""
		if rel.Kinded {
			join = " JOIN relation_kind k ON k.id = l.kind_id"
			if anchorIsA {
				label = "k.inverse_name"
			} else {
				label = "k.name"
			}
		}
		q := fmt.Sprintf(`SELECT l.id, l.%[1]s, %[2]s, %[3]s, %[4]s, %[5]s, %[6]s, %[7]s, %[8]s
			FROM %[9]s l JOIN %[10]s o ON o.id = l.%[1]s%[11]s
			WHERE l.%[12]s = ? AND l.deleted_at IS NULL AND o.archived = 0%[13]s
			ORDER BY %[14]s COALESCE(l.%[15]s, '9999') DESC, 3`,
			otherCol, nameExpr[otherKind], label,
			lcol(st, model.AttrRole), lcol(st, model.AttrFrom), lcol(st, model.AttrTo),
			primaryExpr(st), lcol(st, model.AttrNotes),
			st.table, tableOf[otherKind], join, anchorCol, st.fixedWhere("l."),
			orderPrimary(st), orderDate(st))
		rows, err := r.db.QueryContext(ctx, q, a.ID)
		if err != nil {
			return nil, fmt.Errorf("list %s links of %s: %w", rel.Name, a, err)
		}
		for rows.Next() {
			var li model.LinkedObject
			var primary int
			if err := rows.Scan(&li.LinkID, &li.ID, &li.Name, &li.Label, &li.Role, &li.StartedOn, &li.EndedOn, &primary, &li.Notes); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan link: %w", err)
			}
			li.Relation, li.Kind, li.Primary = rel.Name, otherKind, primary == 1
			li.Incoming = !anchorIsA
			li.Current = model.CurrentOn(li.EndedOn, today)
			out = append(out, li)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func lcol(st store, key string) string {
	if c := st.col(key); c != "NULL" {
		return "l." + c
	}
	return "NULL"
}

func primaryExpr(st store) string {
	if c := st.col(model.AttrPrimary); c != "NULL" {
		return "l." + c
	}
	return "0"
}

func orderPrimary(st store) string {
	if c := st.col(model.AttrPrimary); c != "NULL" {
		if to := lcol(st, model.AttrTo); to != "NULL" {
			return "l." + c + " DESC, CASE WHEN " + to + " IS NULL THEN 0 ELSE 1 END,"
		}
		return "l." + c + " DESC,"
	}
	return ""
}

func orderDate(st store) string {
	if c := st.col(model.AttrFrom); c != "NULL" {
		return c
	}
	return "id"
}

// Roles lists the role values already used in a relation, most used first, for suggestions.
func (r *LinkRepo) Roles(ctx context.Context, relation string, limit int) ([]string, error) {
	rel, ok := model.RelationNamed(relation)
	if !ok || !rel.HasAttr(model.AttrRole) {
		return nil, fmt.Errorf("relation %q has no roles: %w", relation, model.ErrValidation)
	}
	st := storeOf(rel)
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`SELECT role FROM %s WHERE role IS NOT NULL AND role <> '' AND deleted_at IS NULL%s
		GROUP BY lower(role) ORDER BY COUNT(*) DESC, role LIMIT ?`, st.table, st.fixedWhere("")), limit)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// PrimaryOrgName is the name of the person's primary organization, "" when there is none. DAV
// writes it as the vCard ORG.
func PrimaryOrgName(ctx context.Context, q DBTX, personID int64) (string, error) {
	var name string
	err := q.QueryRowContext(ctx, `SELECT o.name FROM membership m JOIN organizations o ON o.id = m.org_id
		WHERE m.person_id = ? AND m.is_primary = 1 AND m.deleted_at IS NULL AND o.archived = 0`, personID).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("primary organization: %w", err)
	}
	return name, nil
}
