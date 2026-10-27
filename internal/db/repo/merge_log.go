package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Persistent merge log (owner decision 2026-10-27, open discrepancy X9). Every person and
// organization merge writes one merge_log row in the same transaction. The row keeps the loser,
// the survivor, a summary of what moved and a journal of every row the merge changed: rows it
// inserted (the merge_alias, copied emails), rows it changed (the loser archived, links repointed,
// survivor fields overwritten, with each changed column before and after) and rows it deleted.
// `crm merge undo <id>` replays the journal backwards in any later session, `crm merge redo <id>`
// forwards. A column that someone changed after the merge is left alone and counted as skipped, so
// an undo never wipes a later edit.

// RowEdit is one row change of a merge. Update edits carry only the changed columns.
type RowEdit struct {
	Table  string         `json:"table"`
	RowID  int64          `json:"rowid"`
	Op     string         `json:"op"` // insert, update, delete
	Cols   []string       `json:"cols,omitempty"`
	Before map[string]any `json:"before,omitempty"`
	After  map[string]any `json:"after,omitempty"`
}

// MergeLogEntry is one logged merge, without its journal.
type MergeLogEntry struct {
	ID           int64          `json:"id"`
	UUID         string         `json:"uuid"`
	Kind         string         `json:"kind"`
	SurvivorID   int64          `json:"survivor_id"`
	SurvivorName string         `json:"survivor_name"`
	LoserIDs     []int64        `json:"loser_ids"`
	LoserNames   []string       `json:"loser_names"`
	Summary      map[string]any `json:"summary"`
	Status       string         `json:"status"`
	CreatedAt    string         `json:"created_at"`
	UndoneAt     *string        `json:"undone_at,omitempty"`
}

// Label is a short description for messages and the undo history.
func (e MergeLogEntry) Label() string {
	return fmt.Sprintf("merge %s into %s", strings.Join(e.LoserNames, ", "), e.SurvivorName)
}

// MergeReplayResult says what an undo or redo did. Skipped counts columns and rows left as they
// are because they changed after the merge.
type MergeReplayResult struct {
	Entry   MergeLogEntry `json:"entry"`
	Skipped int           `json:"skipped"`
}

// MergeLogRepo reads the merge log and replays entries.
type MergeLogRepo struct {
	db *sql.DB
}

// NewMergeLogRepo creates a MergeLogRepo.
func NewMergeLogRepo(db *sql.DB) *MergeLogRepo { return &MergeLogRepo{db: db} }

// mergeTables are the tables a merge journal may name. The journal is read back from the database,
// so table names are checked against this list and column names against the table itself.
var mergeTables = map[string]bool{
	"people": true, "person_emails": true, "person_phones": true, "person_handles": true, "custom_fields": true,
	"taggings": true, "membership": true, "person_relation": true, "deal_person": true, "activity_target": true,
	"merge_alias": true, "dupe_dismissals": true, "organizations": true, "org_relation": true, "deal_org": true,
	"org_dupe_dismissals": true,
}

// snapSpec selects the rows of one table a merge can touch.
type snapSpec struct {
	table, where string
	args         []any
}

type rowSnap map[string]map[int64]map[string]any

func inList(ids []int64) (string, []any) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return placeholders(len(ids)), args
}

func personSnapSpecs(ids []int64) []snapSpec {
	ph, a := inList(ids)
	dup := func(n int) []any {
		var out []any
		for i := 0; i < n; i++ {
			out = append(out, a...)
		}
		return out
	}
	return []snapSpec{
		{"people", "id IN (" + ph + ") OR merged_into IN (" + ph + ")", dup(2)},
		{"person_emails", "person_id IN (" + ph + ")", a},
		{"person_phones", "person_id IN (" + ph + ")", a},
		{"person_handles", "person_id IN (" + ph + ")", a},
		{"custom_fields", "entity_type = 'person' AND entity_id IN (" + ph + ")", a},
		{"taggings", "entity_type = 'person' AND entity_id IN (" + ph + ")", a},
		{"membership", "person_id IN (" + ph + ")", a},
		{"person_relation", "a_id IN (" + ph + ") OR b_id IN (" + ph + ")", dup(2)},
		{"deal_person", "person_id IN (" + ph + ")", a},
		{"activity_target", "target_kind = 'person' AND target_id IN (" + ph + ")", a},
		{"merge_alias", "kind = 'person' AND (survivor_id IN (" + ph + ") OR old_id IN (" + ph + "))", dup(2)},
		{"dupe_dismissals", "person_a IN (" + ph + ") OR person_b IN (" + ph + ")", dup(2)},
	}
}

func orgSnapSpecs(ids []int64) []snapSpec {
	ph, a := inList(ids)
	dup := func(n int) []any {
		var out []any
		for i := 0; i < n; i++ {
			out = append(out, a...)
		}
		return out
	}
	return []snapSpec{
		{"organizations", "id IN (" + ph + ")", a},
		{"membership", "org_id IN (" + ph + ")", a},
		{"org_relation", "a_id IN (" + ph + ") OR b_id IN (" + ph + ")", dup(2)},
		{"deal_org", "org_id IN (" + ph + ")", a},
		{"activity_target", "target_kind = 'organization' AND target_id IN (" + ph + ")", a},
		{"taggings", "entity_type = 'organization' AND entity_id IN (" + ph + ")", a},
		{"merge_alias", "kind = 'organization' AND (survivor_id IN (" + ph + ") OR old_id IN (" + ph + "))", dup(2)},
		{"org_dupe_dismissals", "org_a IN (" + ph + ") OR org_b IN (" + ph + ")", dup(2)},
	}
}

func jsonValue(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

func takeSnapshot(ctx context.Context, q DBTX, specs []snapSpec) (rowSnap, error) {
	out := rowSnap{}
	for _, sp := range specs {
		rows, err := q.QueryContext(ctx, "SELECT rowid AS __rid, * FROM "+sp.table+" WHERE "+sp.where, sp.args...)
		if err != nil {
			return nil, fmt.Errorf("snapshot %s: %w", sp.table, err)
		}
		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			return nil, err
		}
		m := map[int64]map[string]any{}
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return nil, err
			}
			rid, _ := vals[0].(int64)
			row := map[string]any{}
			for i := 1; i < len(cols); i++ {
				row[cols[i]] = jsonValue(vals[i])
			}
			m[rid] = row
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		out[sp.table] = m
	}
	return out, nil
}

func sameValue(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	norm := func(v any) string {
		if f, ok := v.(float64); ok && f == float64(int64(f)) {
			return fmt.Sprint(int64(f))
		}
		return fmt.Sprint(v)
	}
	return norm(a) == norm(b)
}

// diffSnapshots lists the row changes between two snapshots, tables in spec order and rows by id.
// updated_at alone is not a change worth journaling: replays bump it themselves.
func diffSnapshots(before, after rowSnap, specs []snapSpec) []RowEdit {
	var edits []RowEdit
	for _, sp := range specs {
		b, a := before[sp.table], after[sp.table]
		ids := map[int64]bool{}
		for id := range b {
			ids[id] = true
		}
		for id := range a {
			ids[id] = true
		}
		sorted := make([]int64, 0, len(ids))
		for id := range ids {
			sorted = append(sorted, id)
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		for _, id := range sorted {
			br, bok := b[id]
			ar, aok := a[id]
			switch {
			case !bok && aok:
				edits = append(edits, RowEdit{Table: sp.table, RowID: id, Op: "insert", After: ar})
			case bok && !aok:
				edits = append(edits, RowEdit{Table: sp.table, RowID: id, Op: "delete", Before: br})
			default:
				e := RowEdit{Table: sp.table, RowID: id, Op: "update", Before: map[string]any{}, After: map[string]any{}}
				var cols []string
				for c, av := range ar {
					if c == "updated_at" || sameValue(br[c], av) {
						continue
					}
					cols = append(cols, c)
				}
				if len(cols) == 0 {
					continue
				}
				sort.Strings(cols)
				for _, c := range cols {
					e.Before[c], e.After[c] = br[c], ar[c]
				}
				e.Cols = cols
				edits = append(edits, e)
			}
		}
	}
	return edits
}

// writeMergeLog stores one merge and returns its log id. It runs inside the merge transaction.
func writeMergeLog(ctx context.Context, tx *sql.Tx, kind string, survivor int64, survivorName string, losers []int64, loserNames []string, summary any, edits []RowEdit) (int64, error) {
	lj, _ := json.Marshal(losers)
	nj, _ := json.Marshal(loserNames)
	sj, _ := json.Marshal(summary)
	ej, err := json.Marshal(edits)
	if err != nil {
		return 0, fmt.Errorf("encode merge log: %w", err)
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO merge_log (uuid, kind, survivor_id, survivor_name, loser_ids, loser_names, summary, edits)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, uuid.New().String(), kind, survivor, survivorName, string(lj), string(nj), string(sj), string(ej))
	if err != nil {
		return 0, fmt.Errorf("write merge log: %w", err)
	}
	return res.LastInsertId()
}

const mergeLogCols = "id, uuid, kind, survivor_id, survivor_name, loser_ids, loser_names, summary, status, created_at, undone_at"

func scanMergeLog(row interface{ Scan(...any) error }) (*MergeLogEntry, error) {
	var e MergeLogEntry
	var lj, nj, sj string
	if err := row.Scan(&e.ID, &e.UUID, &e.Kind, &e.SurvivorID, &e.SurvivorName, &lj, &nj, &sj, &e.Status, &e.CreatedAt, &e.UndoneAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(lj), &e.LoserIDs)
	_ = json.Unmarshal([]byte(nj), &e.LoserNames)
	_ = json.Unmarshal([]byte(sj), &e.Summary)
	return &e, nil
}

// List returns logged merges, newest first. kind "" lists both kinds, limit 0 lists all.
func (r *MergeLogRepo) List(ctx context.Context, kind string, limit int) ([]MergeLogEntry, error) {
	q := "SELECT " + mergeLogCols + " FROM merge_log"
	var args []any
	if kind != "" {
		q += " WHERE kind = ?"
		args = append(args, kind)
	}
	q += " ORDER BY id DESC"
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list merge log: %w", err)
	}
	defer rows.Close()
	out := []MergeLogEntry{}
	for rows.Next() {
		e, err := scanMergeLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// Get returns one logged merge.
func (r *MergeLogRepo) Get(ctx context.Context, id int64) (*MergeLogEntry, error) {
	e, err := scanMergeLog(r.db.QueryRowContext(ctx, "SELECT "+mergeLogCols+" FROM merge_log WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("merge log entry %d: %w", id, model.ErrNotFound)
	}
	return e, err
}

// Undo reverses a logged merge: the losers come back, their links are repointed back, the alias is
// removed and overwritten survivor fields get their old values.
func (r *MergeLogRepo) Undo(ctx context.Context, id int64) (*MergeReplayResult, error) {
	return r.replay(ctx, id, false)
}

// Redo repeats a merge that Undo reversed.
func (r *MergeLogRepo) Redo(ctx context.Context, id int64) (*MergeReplayResult, error) {
	return r.replay(ctx, id, true)
}

func (r *MergeLogRepo) replay(ctx context.Context, id int64, forward bool) (*MergeReplayResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin merge replay: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var ej string
	row := tx.QueryRowContext(ctx, "SELECT "+mergeLogCols+", edits FROM merge_log WHERE id = ?", id)
	var e MergeLogEntry
	var lj, nj, sj string
	err = row.Scan(&e.ID, &e.UUID, &e.Kind, &e.SurvivorID, &e.SurvivorName, &lj, &nj, &sj, &e.Status, &e.CreatedAt, &e.UndoneAt, &ej)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("merge log entry %d: %w", id, model.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(lj), &e.LoserIDs)
	_ = json.Unmarshal([]byte(nj), &e.LoserNames)
	_ = json.Unmarshal([]byte(sj), &e.Summary)
	switch {
	case !forward && e.Status == "undone":
		return nil, fmt.Errorf("merge %d is already undone (crm merge redo %d repeats it): %w", id, id, model.ErrConflict)
	case forward && e.Status == "merged":
		return nil, fmt.Errorf("merge %d is not undone, so there is nothing to redo: %w", id, model.ErrConflict)
	}
	var edits []RowEdit
	if err := json.Unmarshal([]byte(ej), &edits); err != nil {
		return nil, fmt.Errorf("read merge log %d: %w", id, err)
	}
	rp := &replayer{ctx: ctx, tx: tx, cols: map[string]map[string]bool{}}
	skipped, err := rp.apply(edits, forward)
	if err != nil {
		return nil, err
	}
	if err := rp.touch(edits, e.Kind); err != nil {
		return nil, err
	}
	status, undone := "undone", "datetime('now')"
	if forward {
		status, undone = "merged", "NULL"
	}
	if _, err := tx.ExecContext(ctx, "UPDATE merge_log SET status = ?, undone_at = "+undone+", updated_at = datetime('now') WHERE id = ?", status, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit merge replay: %w", err)
	}
	e.Status = status
	if forward {
		e.UndoneAt = nil
	} else {
		now := "now"
		e.UndoneAt = &now
	}
	return &MergeReplayResult{Entry: e, Skipped: skipped}, nil
}

type replayer struct {
	ctx  context.Context
	tx   *sql.Tx
	cols map[string]map[string]bool
}

func (p *replayer) columns(table string) (map[string]bool, error) {
	if !mergeTables[table] {
		return nil, fmt.Errorf("merge log names an unknown table %q: %w", table, model.ErrValidation)
	}
	if c, ok := p.cols[table]; ok {
		return c, nil
	}
	rows, err := p.tx.QueryContext(p.ctx, "SELECT name FROM pragma_table_info('"+table+"')")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	c := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		c[n] = true
	}
	p.cols[table] = c
	return c, rows.Err()
}

func (p *replayer) current(table, col string, rowid int64) (any, bool, error) {
	var v any
	err := p.tx.QueryRowContext(p.ctx, "SELECT "+col+" FROM "+table+" WHERE rowid = ?", rowid).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	return jsonValue(v), err == nil, err
}

func (p *replayer) apply(edits []RowEdit, forward bool) (int, error) {
	// A merge cannot be undone once the loser was restored or merged elsewhere by hand.
	if !forward {
		for _, e := range edits {
			if (e.Table == "people" || e.Table == "organizations") && e.Op == "update" {
				if want, ok := e.After["archived"]; ok {
					cur, found, err := p.current(e.Table, "archived", e.RowID)
					if err != nil {
						return 0, err
					}
					if !found || !sameValue(cur, want) {
						return 0, fmt.Errorf("%s %d was changed since the merge (restored or merged again), so the merge cannot be undone: %w", e.Table, e.RowID, model.ErrConflict)
					}
				}
			}
		}
	}
	skipped := 0
	n := len(edits)
	for i := 0; i < n; i++ {
		e := edits[i]
		if !forward {
			e = edits[n-1-i]
		}
		cols, err := p.columns(e.Table)
		if err != nil {
			return 0, err
		}
		s, err := p.one(e, cols, forward)
		if err != nil {
			return 0, err
		}
		skipped += s
	}
	return skipped, nil
}

func (p *replayer) one(e RowEdit, cols map[string]bool, forward bool) (int, error) {
	hasDeleted, hasUpdated := cols["deleted_at"], cols["updated_at"]
	bump := ""
	if hasUpdated {
		bump = ", updated_at = datetime('now')"
	}
	insertRow := func(row map[string]any) error {
		names := make([]string, 0, len(row))
		for c := range row {
			if !cols[c] {
				return fmt.Errorf("merge log names an unknown column %s.%s: %w", e.Table, c, model.ErrValidation)
			}
			names = append(names, c)
		}
		sort.Strings(names)
		args := make([]any, len(names))
		for i, c := range names {
			args[i] = row[c]
			if f, ok := args[i].(float64); ok && f == float64(int64(f)) {
				args[i] = int64(f)
			}
		}
		_, err := p.tx.ExecContext(p.ctx, "INSERT OR IGNORE INTO "+e.Table+" ("+strings.Join(names, ", ")+") VALUES ("+placeholders(len(names))+")", args...)
		return err
	}
	_, exists, err := p.current(e.Table, "rowid", e.RowID)
	if err != nil {
		return 0, err
	}
	// An insert goes away on undo and comes back on redo. A delete is the other way round.
	remove := (e.Op == "insert" && !forward) || (e.Op == "delete" && forward)
	restore := (e.Op == "insert" && forward) || (e.Op == "delete" && !forward)
	switch {
	case remove:
		if !exists {
			return 0, nil
		}
		if hasDeleted {
			_, err = p.tx.ExecContext(p.ctx, "UPDATE "+e.Table+" SET deleted_at = datetime('now')"+bump+" WHERE rowid = ? AND deleted_at IS NULL", e.RowID)
		} else {
			_, err = p.tx.ExecContext(p.ctx, "DELETE FROM "+e.Table+" WHERE rowid = ?", e.RowID)
		}
		return 0, err
	case restore:
		row := e.After
		if e.Op == "delete" {
			row = e.Before
		}
		if exists {
			if hasDeleted {
				_, err = p.tx.ExecContext(p.ctx, "UPDATE "+e.Table+" SET deleted_at = NULL"+bump+" WHERE rowid = ?", e.RowID)
			}
			return 0, err
		}
		return 0, insertRow(row)
	}
	// update: a column is written only while it still holds the value the merge left (or found).
	if !exists {
		return len(e.Cols), nil
	}
	from, to := e.After, e.Before
	if forward {
		from, to = e.Before, e.After
	}
	var sets []string
	var args []any
	skipped := 0
	for _, c := range e.Cols {
		if !cols[c] {
			return 0, fmt.Errorf("merge log names an unknown column %s.%s: %w", e.Table, c, model.ErrValidation)
		}
		cur, _, err := p.current(e.Table, c, e.RowID)
		if err != nil {
			return 0, err
		}
		if !sameValue(cur, from[c]) {
			skipped++
			continue
		}
		sets = append(sets, c+" = ?")
		v := to[c]
		if f, ok := v.(float64); ok && f == float64(int64(f)) {
			v = int64(f)
		}
		args = append(args, v)
	}
	if len(sets) == 0 {
		return skipped, nil
	}
	args = append(args, e.RowID)
	_, err = p.tx.ExecContext(p.ctx, "UPDATE "+e.Table+" SET "+strings.Join(sets, ", ")+bump+" WHERE rowid = ?", args...)
	return skipped, err
}

// touch bumps updated_at of the objects a replay changed links of, so sync and DAV see it.
func (p *replayer) touch(edits []RowEdit, kind string) error {
	ids := map[string]map[int64]bool{}
	add := func(table string, id int64) {
		if ids[table] == nil {
			ids[table] = map[int64]bool{}
		}
		ids[table][id] = true
	}
	for _, e := range edits {
		switch e.Table {
		case "people":
			add("people", e.RowID)
		case "organizations":
			add("organizations", e.RowID)
		}
		switch e.Table {
		case "membership", "activity_target", "deal_person", "deal_org":
			p.touchRows(add, e.Table, e)
		}
	}
	for table, set := range ids {
		for id := range set {
			if _, err := p.tx.ExecContext(p.ctx, "UPDATE "+table+" SET updated_at = datetime('now') WHERE id = ?", id); err != nil {
				return err
			}
		}
	}
	return nil
}

// touchRows reads the link row as it is now and marks the objects at its ends.
func (p *replayer) touchRows(add func(string, int64), table string, e RowEdit) {
	switch table {
	case "membership":
		var pid, oid int64
		if p.tx.QueryRowContext(p.ctx, "SELECT person_id, org_id FROM membership WHERE rowid = ?", e.RowID).Scan(&pid, &oid) == nil {
			add("people", pid)
			add("organizations", oid)
		}
	case "deal_person":
		var did, pid int64
		if p.tx.QueryRowContext(p.ctx, "SELECT deal_id, person_id FROM deal_person WHERE rowid = ?", e.RowID).Scan(&did, &pid) == nil {
			add("deals", did)
			add("people", pid)
		}
	case "deal_org":
		var did, oid int64
		if p.tx.QueryRowContext(p.ctx, "SELECT deal_id, org_id FROM deal_org WHERE rowid = ?", e.RowID).Scan(&did, &oid) == nil {
			add("deals", did)
			add("organizations", oid)
		}
	case "activity_target":
		var sk string
		var sid int64
		if p.tx.QueryRowContext(p.ctx, "SELECT source_kind, source_id FROM activity_target WHERE rowid = ?", e.RowID).Scan(&sk, &sid) == nil {
			if sk == "task" {
				add("tasks", sid)
			} else {
				add("interactions", sid)
			}
		}
	}
}
