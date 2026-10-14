package repo

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/dedupe"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// mergeUnit is one scalar field choice. A unit may cover several columns that must move together.
type mergeUnit struct {
	key  string
	cols []string
}

var mergeUnits = []mergeUnit{
	{"first_name", []string{"first_name"}},
	{"last_name", []string{"last_name"}},
	{"title", []string{"title"}},
	{"company", []string{"company"}},
	{"location", []string{"location"}},
	{"notes", []string{"notes"}},
	{"summary", []string{"summary"}},
	{"org_id", []string{"org_id"}},
	{"birthday", []string{"birthday_month", "birthday_day", "birthday_year"}},
	{"keep_in_touch", []string{"keep_in_touch"}},
	{"keep_in_touch_snoozed_until", []string{"keep_in_touch_snoozed_until"}},
	{"last_contacted_at", []string{"last_contacted_at"}},
	{"is_favorite", []string{"is_favorite"}},
	{"priority_tier", []string{"priority_tier"}},
}

// mergeKeyAliases maps accepted choice keys to a unit key.
var mergeKeyAliases = map[string]string{
	"birthday_month": "birthday", "birthday_day": "birthday", "birthday_year": "birthday",
}

// MergeFieldKeys lists every key accepted in the merge choices map besides handle kinds.
func MergeFieldKeys() []string {
	keys := []string{"email", "phone"}
	for _, u := range mergeUnits {
		keys = append(keys, u.key)
	}
	return keys
}

// Merge folds the dropIDs people into keepID in one transaction.
//
// Scalar fields keep the survivor's value unless choices[field] names another person ID. When the
// survivor's value is empty the first non-empty value among the dropped people (in the order given)
// is used. Email, phone, handle, tag and custom field lists are combined and de-duplicated. A
// handle kind keeps the survivor's value unless chosen. Interactions, tasks, task links, deals and
// relationships are re-pointed to the survivor. Each dropped person is archived with merged_into
// set to keepID. Their own rows (fields, emails, phones, handles, tags) are left untouched so the
// merge can be undone in principle.
//
// TODO(F12): record the merge in the undo journal so dropped people can be restored with the
// records that were re-pointed (interactions, tasks, deals, relationships).
func (r *PersonRepo) Merge(ctx context.Context, keepID int64, dropIDs []int64, choices map[string]int64) (*model.MergeResult, error) {
	return r.merge(ctx, keepID, dropIDs, choices, false)
}

// MergeDryRun runs the merge and rolls it back, so the result is exactly what Merge would do.
func (r *PersonRepo) MergeDryRun(ctx context.Context, keepID int64, dropIDs []int64, choices map[string]int64) (*model.MergeResult, error) {
	return r.merge(ctx, keepID, dropIDs, choices, true)
}

func (r *PersonRepo) merge(ctx context.Context, keepID int64, dropIDs []int64, choices map[string]int64, dryRun bool) (*model.MergeResult, error) {
	if len(dropIDs) == 0 {
		return nil, fmt.Errorf("merge needs at least one person to drop: %w", model.ErrValidation)
	}
	involved := map[int64]bool{keepID: true}
	for _, d := range dropIDs {
		if involved[d] {
			return nil, fmt.Errorf("person %d listed twice or is also the survivor: %w", d, model.ErrValidation)
		}
		involved[d] = true
	}
	norm := map[string]int64{}
	for k, v := range choices {
		k = strings.ToLower(strings.TrimSpace(k))
		if a, ok := mergeKeyAliases[k]; ok {
			k = a
		}
		if !validMergeKey(k) {
			return nil, fmt.Errorf("unknown merge field %q (valid: %s, or a handle kind): %w", k, strings.Join(MergeFieldKeys(), ", "), model.ErrValidation)
		}
		if !involved[v] {
			return nil, fmt.Errorf("choice %s=%d is not the survivor or a dropped person: %w", k, v, model.ErrValidation)
		}
		norm[k] = v
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin merge: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	m := &merger{
		ctx: ctx, tx: tx, keepID: keepID, dropIDs: dropIDs, choices: norm,
		res: &model.MergeResult{
			KeepID: keepID, DropIDs: dropIDs, DryRun: dryRun,
			Fields: []model.MergeFieldChange{},
			Added:  map[string]int{"emails": 0, "phones": 0, "handles": 0, "tags": 0, "custom_fields": 0},
			Moved:  map[string]int{"interactions": 0, "tasks": 0, "task_people": 0, "deals": 0, "relationships": 0},
		},
	}
	if err := m.run(); err != nil {
		return nil, err
	}
	if dryRun {
		return m.res, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit merge: %w", err)
	}
	return m.res, nil
}

func validMergeKey(k string) bool {
	if k == "email" || k == "phone" || model.ValidHandleKind(k) {
		return true
	}
	for _, u := range mergeUnits {
		if u.key == k {
			return true
		}
	}
	return false
}

type merger struct {
	ctx     context.Context
	tx      *sql.Tx
	keepID  int64
	dropIDs []int64
	choices map[string]int64
	res     *model.MergeResult
	vals    map[int64]map[string]any
}

func (m *merger) all() []int64 {
	return append([]int64{m.keepID}, m.dropIDs...)
}

func (m *merger) run() error {
	if err := m.load(); err != nil {
		return err
	}
	if err := m.scalars(); err != nil {
		return err
	}
	if err := m.emails(); err != nil {
		return err
	}
	if err := m.phones(); err != nil {
		return err
	}
	if err := m.handles(); err != nil {
		return err
	}
	if err := m.copyLists(); err != nil {
		return err
	}
	if err := m.repoint(); err != nil {
		return err
	}
	if err := m.relationships(); err != nil {
		return err
	}
	return m.archive()
}

func (m *merger) exec(query string, args ...any) (int, error) {
	res, err := m.tx.ExecContext(m.ctx, query, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// load reads every scalar column of every involved person and verifies they are active.
func (m *merger) load() error {
	var cols []string
	for _, u := range mergeUnits {
		cols = append(cols, u.cols...)
	}
	cols = append(cols, "email", "phone")
	m.vals = map[int64]map[string]any{}
	for _, id := range m.all() {
		dest := make([]any, len(cols)+1)
		ptrs := make([]any, len(cols)+1)
		for i := range dest {
			ptrs[i] = &dest[i]
		}
		q := "SELECT " + strings.Join(cols, ", ") + ", archived FROM people WHERE id = ?"
		err := m.tx.QueryRowContext(m.ctx, q, id).Scan(ptrs...)
		if err == sql.ErrNoRows {
			return fmt.Errorf("person %d: %w", id, model.ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("load person %d: %w", id, err)
		}
		if a, _ := dest[len(cols)].(int64); a != 0 {
			return fmt.Errorf("person %d is archived: %w", id, model.ErrValidation)
		}
		v := map[string]any{}
		for i, c := range cols {
			v[c] = dest[i]
		}
		m.vals[id] = v
	}
	return nil
}

func isEmptyVal(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case int64:
		return x == 0
	}
	return false
}

// unitEmpty reports whether the survivor value of a unit counts as empty.
func (m *merger) unitEmpty(id int64, u mergeUnit) bool {
	if u.key == "birthday" {
		return isEmptyVal(m.vals[id]["birthday_month"]) || isEmptyVal(m.vals[id]["birthday_day"])
	}
	return isEmptyVal(m.vals[id][u.cols[0]])
}

func (m *merger) unitValue(id int64, u mergeUnit) any {
	if u.key == "birthday" {
		v := m.vals[id]
		if isEmptyVal(v["birthday_month"]) {
			return nil
		}
		s := fmt.Sprintf("--%02d-%02d", v["birthday_month"], v["birthday_day"])
		if y, ok := v["birthday_year"].(int64); ok {
			s = fmt.Sprintf("%04d-%02d-%02d", y, v["birthday_month"], v["birthday_day"])
		}
		return s
	}
	return m.vals[id][u.cols[0]]
}

func (m *merger) scalars() error {
	var sets []string
	var args []any
	for _, u := range mergeUnits {
		src := m.keepID
		if c, ok := m.choices[u.key]; ok {
			src = c
		} else if m.unitEmpty(m.keepID, u) {
			for _, d := range m.dropIDs {
				if !m.unitEmpty(d, u) {
					src = d
					break
				}
			}
		}
		if src == m.keepID {
			continue
		}
		same := true
		for _, c := range u.cols {
			if fmt.Sprint(m.vals[src][c]) != fmt.Sprint(m.vals[m.keepID][c]) {
				same = false
			}
		}
		if same {
			continue
		}
		for _, c := range u.cols {
			sets = append(sets, c+" = ?") // c comes from the fixed mergeUnits table
			args = append(args, m.vals[src][c])
		}
		m.res.Fields = append(m.res.Fields, model.MergeFieldChange{
			Field: u.key, From: src, Old: m.unitValue(m.keepID, u), New: m.unitValue(src, u),
		})
	}
	sets = append(sets, "updated_at = datetime('now')")
	args = append(args, m.keepID)
	_, err := m.exec("UPDATE people SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	if err != nil {
		return fmt.Errorf("update survivor: %w", err)
	}
	return nil
}

type contactRow struct {
	value string
	label sql.NullString
}

// contacts lists a person's emails or phones, primary first, including the legacy single column.
func (m *merger) contacts(id int64, table, col, legacy string) ([]contactRow, error) {
	q := fmt.Sprintf("SELECT %s, label FROM %s WHERE person_id = ? ORDER BY is_primary DESC, id", col, table)
	rows, err := m.tx.QueryContext(m.ctx, q, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contactRow
	for rows.Next() {
		var c contactRow
		if err := rows.Scan(&c.value, &c.label); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if s, ok := m.vals[id][legacy].(string); ok && strings.TrimSpace(s) != "" {
		found := false
		for _, c := range out {
			if c.value == s {
				found = true
			}
		}
		if !found {
			out = append([]contactRow{{value: s}}, out...)
		}
	}
	return out, nil
}

func (m *merger) emails() error {
	return m.combineContacts("emails", "person_emails", "email", "email", func(s string) string { return dedupe.EmailKey(s) })
}

func (m *merger) phones() error {
	return m.combineContacts("phones", "person_phones", "phone_number", "phone", func(s string) string {
		if k := dedupe.PhoneKey(s); k != "" {
			return k
		}
		return strings.TrimSpace(s)
	})
}

// combineContacts copies the dropped people's emails or phones to the survivor, skipping
// values the survivor already has, then sets the primary.
func (m *merger) combineContacts(name, table, col, legacy string, key func(string) string) error {
	keepRows, err := m.contacts(m.keepID, table, col, legacy)
	if err != nil {
		return fmt.Errorf("list %s: %w", name, err)
	}
	have := map[string]bool{}
	for _, c := range keepRows {
		have[key(c.value)] = true
	}
	primaryOf := map[int64]string{m.keepID: ""}
	if len(keepRows) > 0 {
		primaryOf[m.keepID] = keepRows[0].value
	}
	for _, d := range m.dropIDs {
		rows, err := m.contacts(d, table, col, legacy)
		if err != nil {
			return fmt.Errorf("list %s: %w", name, err)
		}
		if len(rows) > 0 {
			primaryOf[d] = rows[0].value
		}
		for _, c := range rows {
			k := key(c.value)
			if k == "" || have[k] {
				continue
			}
			have[k] = true
			ins := fmt.Sprintf("INSERT OR IGNORE INTO %s (person_id, %s, label, is_primary) VALUES (?, ?, ?, 0)", table, col)
			if _, err := m.exec(ins, m.keepID, c.value, c.label); err != nil {
				return fmt.Errorf("add %s: %w", name, err)
			}
			m.res.Added[name]++
		}
	}

	field := legacy
	cur := primaryOf[m.keepID]
	target := cur
	src := m.keepID
	if c, ok := m.choices[field]; ok {
		src = c
		target = primaryOf[c]
	} else if target == "" {
		for _, d := range m.dropIDs {
			if primaryOf[d] != "" {
				src, target = d, primaryOf[d]
				break
			}
		}
	}
	if target == "" {
		return nil
	}
	// The chosen value may duplicate an existing one that differs only in case or format.
	var exists int
	if err := m.tx.QueryRowContext(m.ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE person_id = ? AND %s = ?", table, col), m.keepID, target).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		ins := fmt.Sprintf("INSERT INTO %s (person_id, %s, is_primary) VALUES (?, ?, 0)", table, col)
		if _, err := m.exec(ins, m.keepID, target); err != nil {
			return fmt.Errorf("add %s: %w", name, err)
		}
	}
	if _, err := m.exec(fmt.Sprintf("UPDATE %s SET is_primary = (%s = ?) WHERE person_id = ?", table, col), target, m.keepID); err != nil {
		return fmt.Errorf("set primary %s: %w", legacy, err)
	}
	// people.email and people.phone mirror the primary, as in syncPrimary.
	if _, err := m.exec("UPDATE people SET "+legacy+" = ?, updated_at = datetime('now') WHERE id = ?", target, m.keepID); err != nil {
		return fmt.Errorf("set primary %s: %w", legacy, err)
	}
	if target != cur {
		m.res.Fields = append(m.res.Fields, model.MergeFieldChange{Field: legacy, From: src, Old: nullable(cur), New: target})
	}
	return nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (m *merger) handleMap(id int64) (map[string]string, error) {
	rows, err := m.tx.QueryContext(m.ctx, "SELECT kind, value FROM person_handles WHERE person_id = ?", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (m *merger) handles() error {
	byPerson := map[int64]map[string]string{}
	for _, id := range m.all() {
		h, err := m.handleMap(id)
		if err != nil {
			return fmt.Errorf("list handles: %w", err)
		}
		byPerson[id] = h
	}
	kinds := append([]string(nil), model.HandleKinds...)
	sort.Strings(kinds)
	for _, kind := range kinds {
		cur, hasCur := byPerson[m.keepID][kind]
		target, src := cur, m.keepID
		if c, ok := m.choices[kind]; ok {
			v, has := byPerson[c][kind]
			if !has {
				return fmt.Errorf("person %d has no %s handle: %w", c, kind, model.ErrValidation)
			}
			target, src = v, c
		} else if !hasCur {
			for _, d := range m.dropIDs {
				if v, ok := byPerson[d][kind]; ok {
					target, src = v, d
					break
				}
			}
		}
		if src == m.keepID || (hasCur && target == cur) {
			continue
		}
		if _, err := m.exec(`INSERT INTO person_handles (person_id, kind, value) VALUES (?, ?, ?)
			ON CONFLICT(person_id, kind) DO UPDATE SET value = excluded.value`, m.keepID, kind, target); err != nil {
			return fmt.Errorf("set handle: %w", err)
		}
		if hasCur {
			m.res.Fields = append(m.res.Fields, model.MergeFieldChange{Field: kind, From: src, Old: cur, New: target})
		} else {
			m.res.Added["handles"]++
		}
	}
	return nil
}

// copyLists copies tags and custom field values. The survivor wins on a custom field name clash.
func (m *merger) copyLists() error {
	for _, d := range m.dropIDs {
		n, err := m.exec(`INSERT OR IGNORE INTO taggings (tag_id, entity_type, entity_id)
			SELECT tag_id, 'person', ? FROM taggings WHERE entity_type = 'person' AND entity_id = ?`, m.keepID, d)
		if err != nil {
			return fmt.Errorf("copy tags: %w", err)
		}
		m.res.Added["tags"] += n
		n, err = m.exec(`INSERT OR IGNORE INTO custom_fields (entity_type, entity_id, field_name, field_value)
			SELECT 'person', ?, field_name, field_value FROM custom_fields WHERE entity_type = 'person' AND entity_id = ?`, m.keepID, d)
		if err != nil {
			return fmt.Errorf("copy custom fields: %w", err)
		}
		m.res.Added["custom_fields"] += n
	}
	return nil
}

// repoint moves interactions, task links, tasks and deals to the survivor.
func (m *merger) repoint() error {
	for _, d := range m.dropIDs {
		if _, err := m.exec(`UPDATE interactions SET updated_at = datetime('now')
			WHERE id IN (SELECT interaction_id FROM interaction_people WHERE person_id = ?)`, d); err != nil {
			return fmt.Errorf("touch interactions: %w", err)
		}
		n, err := m.exec(`UPDATE OR IGNORE interaction_people SET person_id = ? WHERE person_id = ?`, m.keepID, d)
		if err != nil {
			return fmt.Errorf("move interactions: %w", err)
		}
		m.res.Moved["interactions"] += n

		if _, err := m.exec(`UPDATE tasks SET updated_at = datetime('now')
			WHERE id IN (SELECT task_id FROM task_people WHERE person_id = ?)`, d); err != nil {
			return fmt.Errorf("touch tasks: %w", err)
		}
		n, err = m.exec(`UPDATE OR IGNORE task_people SET person_id = ? WHERE person_id = ?`, m.keepID, d)
		if err != nil {
			return fmt.Errorf("move task links: %w", err)
		}
		m.res.Moved["task_people"] += n
		n, err = m.exec(`UPDATE tasks SET person_id = ?, updated_at = datetime('now') WHERE person_id = ?`, m.keepID, d)
		if err != nil {
			return fmt.Errorf("move tasks: %w", err)
		}
		m.res.Moved["tasks"] += n
		n, err = m.exec(`UPDATE deals SET person_id = ?, updated_at = datetime('now') WHERE person_id = ?`, m.keepID, d)
		if err != nil {
			return fmt.Errorf("move deals: %w", err)
		}
		m.res.Moved["deals"] += n
	}
	return nil
}

// relationships re-points links that involve a dropped person. Self links and duplicates are left
// on the dropped person.
func (m *merger) relationships() error {
	drop := map[int64]bool{}
	for _, d := range m.dropIDs {
		drop[d] = true
	}
	type rel struct {
		id, a, b int64
		typ      string
	}
	var rels []rel
	for _, d := range m.dropIDs {
		rows, err := m.tx.QueryContext(m.ctx,
			"SELECT id, person_id, related_person_id, type FROM relationships WHERE person_id = ? OR related_person_id = ?", d, d)
		if err != nil {
			return fmt.Errorf("list relationships: %w", err)
		}
		for rows.Next() {
			var r rel
			if err := rows.Scan(&r.id, &r.a, &r.b, &r.typ); err != nil {
				_ = rows.Close()
				return err
			}
			rels = append(rels, r)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		_ = rows.Close()
	}
	seen := map[int64]bool{}
	for _, r := range rels {
		if seen[r.id] {
			continue
		}
		seen[r.id] = true
		a, b := r.a, r.b
		if drop[a] {
			a = m.keepID
		}
		if drop[b] {
			b = m.keepID
		}
		if a == b {
			continue
		}
		var exists int
		if err := m.tx.QueryRowContext(m.ctx,
			"SELECT COUNT(*) FROM relationships WHERE person_id = ? AND related_person_id = ? AND type = ?", a, b, r.typ).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		if _, err := m.exec("UPDATE relationships SET person_id = ?, related_person_id = ? WHERE id = ?", a, b, r.id); err != nil {
			return fmt.Errorf("move relationship: %w", err)
		}
		m.res.Moved["relationships"]++
	}
	return nil
}

// archive soft-deletes the dropped people and clears dismissals between the merged people.
func (m *merger) archive() error {
	for _, d := range m.dropIDs {
		if _, err := m.exec(`UPDATE people SET archived = 1, merged_into = ?, updated_at = datetime('now') WHERE id = ?`, m.keepID, d); err != nil {
			return fmt.Errorf("archive person %d: %w", d, err)
		}
		// Earlier merges into this person now point at the survivor.
		if _, err := m.exec(`UPDATE people SET merged_into = ?, updated_at = datetime('now') WHERE merged_into = ?`, m.keepID, d); err != nil {
			return fmt.Errorf("repoint merged_into: %w", err)
		}
	}
	ids := m.all()
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, 2*len(ids))
	for i := 0; i < 2; i++ {
		for _, id := range ids {
			args = append(args, id)
		}
	}
	_, err := m.exec(`UPDATE dupe_dismissals SET archived = 1, updated_at = datetime('now')
		WHERE archived = 0 AND person_a IN (`+ph+`) AND person_b IN (`+ph+`)`, args...)
	if err != nil {
		return fmt.Errorf("clear dismissals: %w", err)
	}
	return nil
}
