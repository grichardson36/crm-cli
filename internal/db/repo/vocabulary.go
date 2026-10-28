package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/validate"
)

// VocabRepo manages the user-managed vocabularies (table vocabulary, registry model.Vocabularies).
// It is kind-agnostic: deal roles use it today, deal stages and note types can move onto it later by
// registering a kind. Every door (CLI, MCP, TUI) calls these functions.
type VocabRepo struct{ db *sql.DB }

// NewVocabRepo creates a VocabRepo.
func NewVocabRepo(db *sql.DB) *VocabRepo { return &VocabRepo{db: db} }

const vocabCols = "id, uuid, kind, name, position, archived, created_at, updated_at"

type rowScanner interface{ Scan(...any) error }

func scanVocab(r rowScanner) (*model.VocabEntry, error) {
	var e model.VocabEntry
	if err := r.Scan(&e.ID, &e.UUID, &e.Kind, &e.Name, &e.Position, &e.Removed, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, err
	}
	return &e, nil
}

func vocabSpec(kind string) (model.VocabSpec, error) {
	vs, ok := model.VocabularyNamed(kind)
	if !ok {
		return vs, fmt.Errorf("unknown vocabulary %q: %w", kind, model.ErrValidation)
	}
	return vs, nil
}

type vocabQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func usageWhere(u model.VocabUsage) string {
	w := fmt.Sprintf("%s = ? COLLATE NOCASE", u.Column)
	if u.Live != "" {
		w += " AND " + u.Live
	}
	return w
}

// usage counts the live rows that hold the text of a value.
func usage(ctx context.Context, q vocabQuerier, vs model.VocabSpec, name string) (int, error) {
	total := 0
	for _, u := range vs.Usages {
		var n int
		if err := q.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", u.Table, usageWhere(u)), name).Scan(&n); err != nil {
			return 0, fmt.Errorf("count %s use: %w", vs.Noun, err)
		}
		total += n
	}
	return total, nil
}

func findVocab(ctx context.Context, q vocabQuerier, kind, name string) (*model.VocabEntry, error) {
	e, err := scanVocab(q.QueryRowContext(ctx, "SELECT "+vocabCols+" FROM vocabulary WHERE kind = ? AND name = ?", kind, model.CleanVocabName(name)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%s %q: %w", kind, name, model.ErrNotFound)
	}
	return e, err
}

func vocabNotFound(vs model.VocabSpec, name string) error {
	return fmt.Errorf("%s %q is not on the list (see `%s list`): %w", vs.Noun, name, vs.Command, model.ErrNotFound)
}

// List returns the values of a vocabulary in pick order with the number of records that use each.
// Removed values are included (after the live ones) only when all is true.
func (r *VocabRepo) List(ctx context.Context, kind string, all bool) ([]*model.VocabEntry, error) {
	vs, err := vocabSpec(kind)
	if err != nil {
		return nil, err
	}
	q := "SELECT " + vocabCols + " FROM vocabulary WHERE kind = ?"
	if !all {
		q += " AND archived = 0"
	}
	rows, err := r.db.QueryContext(ctx, q+" ORDER BY archived, position, id", kind)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", vs.Label, err)
	}
	var out []*model.VocabEntry
	for rows.Next() {
		e, err := scanVocab(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, e := range out {
		if e.InUse, err = usage(ctx, r.db, vs, e.Name); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Names lists the live values of a vocabulary in pick order.
func (r *VocabRepo) Names(ctx context.Context, kind string) ([]string, error) {
	if _, err := vocabSpec(kind); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, "SELECT name FROM vocabulary WHERE kind = ? AND archived = 0 ORDER BY position, id", kind)
	if err != nil {
		return nil, fmt.Errorf("list vocabulary: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Field returns a declared choice field backed by a vocabulary with its options set to the live
// values, in pick order. fieldKind and key are the model.Fields entry ("deal_person", "role").
func (r *VocabRepo) Field(ctx context.Context, fieldKind, key string) (model.FieldSpec, error) {
	spec, ok := model.FieldNamed(fieldKind, key)
	if !ok || spec.Vocabulary == "" {
		return spec, fmt.Errorf("%s.%s is not a vocabulary field: %w", fieldKind, key, model.ErrValidation)
	}
	names, err := r.Names(ctx, spec.Vocabulary)
	if err != nil {
		return spec, err
	}
	return spec.WithVocabulary(names), nil
}

// Resolve checks a typed value against a vocabulary field through internal/validate. It returns the
// listed spelling. An unknown value is E_VOCAB_UNKNOWN, unless create is true: then the value is
// added to the list and created is true. An empty value passes and clears the field.
func (r *VocabRepo) Resolve(ctx context.Context, fieldKind, key, raw string, create bool) (name string, created bool, err error) {
	spec, err := r.Field(ctx, fieldKind, key)
	if err != nil {
		return "", false, err
	}
	res := validate.Field(spec, raw, validate.Options{Create: create})
	if err := res.Err(); err != nil {
		return "", false, err
	}
	for _, s := range res.Suggestions {
		if s.Code == validate.CodeVocabCreate {
			if _, err := r.Add(ctx, spec.Vocabulary, res.Value); err != nil {
				return "", false, err
			}
			return res.Value, true, nil
		}
	}
	return res.Value, false, nil
}

func nextPosition(ctx context.Context, q vocabQuerier, kind string) (int, error) {
	var p sql.NullInt64
	if err := q.QueryRowContext(ctx, "SELECT MAX(position) FROM vocabulary WHERE kind = ? AND archived = 0", kind).Scan(&p); err != nil {
		return 0, err
	}
	return int(p.Int64) + 1, nil
}

// Add puts a value at the end of the list. A name that is already listed (any case) is
// E_VOCAB_DUPLICATE. A removed value of the same name comes back at the end.
func (r *VocabRepo) Add(ctx context.Context, kind, name string) (*model.VocabEntry, error) {
	if _, err := vocabSpec(kind); err != nil {
		return nil, err
	}
	name, err := validate.VocabName(kind, name)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	e, err := addIn(ctx, tx, kind, name)
	if err != nil {
		return nil, err
	}
	return e, tx.Commit()
}

func addIn(ctx context.Context, tx vocabQuerier, kind, name string) (*model.VocabEntry, error) {
	pos, err := nextPosition(ctx, tx, kind)
	if err != nil {
		return nil, err
	}
	ex, err := findVocab(ctx, tx, kind, name)
	switch {
	case err == nil:
		if !ex.Removed {
			return nil, validate.VocabDuplicate(kind, ex.Name)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE vocabulary SET archived = 0, name = ?, position = ?, updated_at = datetime('now') WHERE id = ?", name, pos, ex.ID); err != nil {
			return nil, fmt.Errorf("restore value: %w", err)
		}
		return findVocab(ctx, tx, kind, name)
	case !errors.Is(err, model.ErrNotFound):
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO vocabulary (uuid, kind, name, position) VALUES (?, ?, ?, ?)", uuid.New().String(), kind, name, pos); err != nil {
		return nil, fmt.Errorf("add value: %w", err)
	}
	return findVocab(ctx, tx, kind, name)
}

// RenameResult says what a rename did.
type RenameResult struct {
	Old string `json:"old"`
	New string `json:"new"`
	// Rows is how many records had the text and now have the new one.
	Rows int `json:"rows"`
}

// Rename changes a value and rewrites the records that hold it (all of them, so a restored link
// agrees). Renaming to the spelling of another listed value is E_VOCAB_DUPLICATE. Changing only the
// case of a value is a rename too.
func (r *VocabRepo) Rename(ctx context.Context, kind, from, to string) (RenameResult, error) {
	vs, err := vocabSpec(kind)
	if err != nil {
		return RenameResult{}, err
	}
	to, err = validate.VocabName(kind, to)
	if err != nil {
		return RenameResult{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return RenameResult{}, err
	}
	defer tx.Rollback()
	cur, err := findVocab(ctx, tx, kind, from)
	if err != nil || cur.Removed {
		return RenameResult{}, vocabNotFound(vs, from)
	}
	if other, err := findVocab(ctx, tx, kind, to); err == nil && other.ID != cur.ID {
		if !other.Removed {
			return RenameResult{}, validate.VocabDuplicate(kind, other.Name)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM vocabulary WHERE id = ?", other.ID); err != nil {
			return RenameResult{}, err
		}
	}
	res := RenameResult{Old: cur.Name, New: to}
	if _, err := tx.ExecContext(ctx, "UPDATE vocabulary SET name = ?, updated_at = datetime('now') WHERE id = ?", to, cur.ID); err != nil {
		return RenameResult{}, fmt.Errorf("rename value: %w", err)
	}
	for _, u := range vs.Usages {
		out, err := tx.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s = ?, updated_at = datetime('now') WHERE %s = ? COLLATE NOCASE", u.Table, u.Column, u.Column), to, cur.Name)
		if err != nil {
			return RenameResult{}, fmt.Errorf("rewrite %s.%s: %w", u.Table, u.Column, err)
		}
		n, _ := out.RowsAffected()
		res.Rows += int(n)
	}
	return res, tx.Commit()
}

// RemoveResult says what a removal did.
type RemoveResult struct {
	Name string `json:"name"`
	// Moved is how many records were given the replacement. Kept is how many still hold the removed
	// text as plain text.
	Moved      int    `json:"moved"`
	Kept       int    `json:"kept"`
	ReplacedBy string `json:"replaced_by,omitempty"`
}

// Remove takes a value off the list. When records still use it, the call needs a replacement
// (they are moved to it) or keep (they keep the text, shown as plain text and no longer a listed
// value). Neither is E_VOCAB_IN_USE. Nothing in the records is lost either way.
func (r *VocabRepo) Remove(ctx context.Context, kind, name, replaceWith string, keep bool) (RemoveResult, error) {
	vs, err := vocabSpec(kind)
	if err != nil {
		return RemoveResult{}, err
	}
	if replaceWith != "" && keep {
		return RemoveResult{}, fmt.Errorf("use a replacement or keep, not both: %w", model.ErrValidation)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return RemoveResult{}, err
	}
	defer tx.Rollback()
	cur, err := findVocab(ctx, tx, kind, name)
	if err != nil || cur.Removed {
		return RemoveResult{}, vocabNotFound(vs, name)
	}
	n, err := usage(ctx, tx, vs, cur.Name)
	if err != nil {
		return RemoveResult{}, err
	}
	res := RemoveResult{Name: cur.Name}
	switch {
	case n > 0 && replaceWith == "" && !keep:
		return RemoveResult{}, validate.VocabInUse(kind, cur.Name, n)
	case replaceWith != "":
		rep, err := findVocab(ctx, tx, kind, replaceWith)
		if err != nil || rep.Removed {
			return RemoveResult{}, vocabNotFound(vs, replaceWith)
		}
		if rep.ID == cur.ID {
			return RemoveResult{}, fmt.Errorf("the replacement must be another %s: %w", vs.Noun, model.ErrValidation)
		}
		for _, u := range vs.Usages {
			out, err := tx.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s = ?, updated_at = datetime('now') WHERE %s = ? COLLATE NOCASE", u.Table, u.Column, u.Column), rep.Name, cur.Name)
			if err != nil {
				return RemoveResult{}, fmt.Errorf("move %s.%s: %w", u.Table, u.Column, err)
			}
			c, _ := out.RowsAffected()
			res.Moved += int(c)
		}
		res.ReplacedBy = rep.Name
	default:
		res.Kept = n
	}
	if _, err := tx.ExecContext(ctx, "UPDATE vocabulary SET archived = 1, updated_at = datetime('now') WHERE id = ?", cur.ID); err != nil {
		return RemoveResult{}, fmt.Errorf("remove value: %w", err)
	}
	return res, tx.Commit()
}

func (r *VocabRepo) live(ctx context.Context, kind string) ([]*model.VocabEntry, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT "+vocabCols+" FROM vocabulary WHERE kind = ? AND archived = 0 ORDER BY position, id", kind)
	if err != nil {
		return nil, fmt.Errorf("list vocabulary: %w", err)
	}
	defer rows.Close()
	var out []*model.VocabEntry
	for rows.Next() {
		e, err := scanVocab(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Reorder puts the named values first in the given order. Values not named keep their order after
// them. A name that is not on the list is an error and nothing changes.
func (r *VocabRepo) Reorder(ctx context.Context, kind string, names []string) error {
	vs, err := vocabSpec(kind)
	if err != nil {
		return err
	}
	cur, err := r.live(ctx, kind)
	if err != nil {
		return err
	}
	byName := map[string]*model.VocabEntry{}
	for _, e := range cur {
		byName[strings.ToLower(e.Name)] = e
	}
	var order []*model.VocabEntry
	seen := map[int64]bool{}
	for _, n := range names {
		e, ok := byName[strings.ToLower(model.CleanVocabName(n))]
		if !ok {
			return vocabNotFound(vs, n)
		}
		if !seen[e.ID] {
			seen[e.ID] = true
			order = append(order, e)
		}
	}
	for _, e := range cur {
		if !seen[e.ID] {
			order = append(order, e)
		}
	}
	return r.setOrder(ctx, order)
}

// Move puts one value at a 1-based position, the others keep their order.
func (r *VocabRepo) Move(ctx context.Context, kind, name string, position int) error {
	vs, err := vocabSpec(kind)
	if err != nil {
		return err
	}
	if position < 1 {
		return fmt.Errorf("position starts at 1: %w", model.ErrValidation)
	}
	cur, err := r.live(ctx, kind)
	if err != nil {
		return err
	}
	idx := -1
	for i, e := range cur {
		if strings.EqualFold(e.Name, model.CleanVocabName(name)) {
			idx = i
		}
	}
	if idx < 0 {
		return vocabNotFound(vs, name)
	}
	e := cur[idx]
	rest := append(append([]*model.VocabEntry{}, cur[:idx]...), cur[idx+1:]...)
	if position > len(rest)+1 {
		position = len(rest) + 1
	}
	order := append(append(append([]*model.VocabEntry{}, rest[:position-1]...), e), rest[position-1:]...)
	return r.setOrder(ctx, order)
}

func (r *VocabRepo) setOrder(ctx context.Context, order []*model.VocabEntry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, e := range order {
		if e.Position == i+1 {
			continue
		}
		if _, err := tx.ExecContext(ctx, "UPDATE vocabulary SET position = ?, updated_at = datetime('now') WHERE id = ?", i+1, e.ID); err != nil {
			return fmt.Errorf("reorder: %w", err)
		}
	}
	return tx.Commit()
}

// --- backfill: free-text values already in the records become listed values ---

// VocabCandidate is one distinct value found in the records that is not on the list.
type VocabCandidate struct {
	Name string `json:"name"`
	Rows int    `json:"rows"`
}

// VocabBackfillReport is what a backfill would do, or did.
type VocabBackfillReport struct {
	Kind string `json:"kind"`
	// Rows is how many records hold a value. Distinct is the number of different values (case ignored).
	Rows     int `json:"rows"`
	Distinct int `json:"distinct"`
	// Listed values are already on the list. Removed are on the list as removed values and stay
	// plain text on purpose. Invalid cannot be a list value (too long, a pipe).
	Listed  int              `json:"already_listed"`
	Removed int              `json:"removed_skipped"`
	Invalid int              `json:"invalid_skipped"`
	ToAdd   []VocabCandidate `json:"to_add"`
}

// Line is the one-line summary.
func (b VocabBackfillReport) Line() string {
	return fmt.Sprintf("%d records hold a value, %d distinct, %d already listed, %d removed on purpose, %d unusable, %d to add",
		b.Rows, b.Distinct, b.Listed, b.Removed, b.Invalid, len(b.ToAdd))
}

// BackfillPlan reads the values the records hold and says which are not on the list. It writes nothing.
func (r *VocabRepo) BackfillPlan(ctx context.Context, kind string) (VocabBackfillReport, error) {
	vs, err := vocabSpec(kind)
	if err != nil {
		return VocabBackfillReport{}, err
	}
	rep := VocabBackfillReport{Kind: kind, ToAdd: []VocabCandidate{}}
	all, err := r.List(ctx, kind, true)
	if err != nil {
		return rep, err
	}
	listed := map[string]bool{}
	removed := map[string]bool{}
	for _, e := range all {
		if e.Removed {
			removed[strings.ToLower(e.Name)] = true
		} else {
			listed[strings.ToLower(e.Name)] = true
		}
	}
	type group struct {
		spell map[string]int
		rows  int
	}
	groups := map[string]*group{}
	for _, u := range vs.Usages {
		live := ""
		if u.Live != "" {
			live = " AND " + u.Live
		}
		rows, err := r.db.QueryContext(ctx, fmt.Sprintf("SELECT trim(%s), COUNT(*) FROM %s WHERE %s IS NOT NULL AND trim(%s) <> ''%s GROUP BY trim(%s)", u.Column, u.Table, u.Column, u.Column, live, u.Column))
		if err != nil {
			return rep, fmt.Errorf("read %s.%s: %w", u.Table, u.Column, err)
		}
		for rows.Next() {
			var v string
			var n int
			if err := rows.Scan(&v, &n); err != nil {
				rows.Close()
				return rep, err
			}
			v = model.CleanVocabName(v)
			g := groups[strings.ToLower(v)]
			if g == nil {
				g = &group{spell: map[string]int{}}
				groups[strings.ToLower(v)] = g
			}
			g.spell[v] += n
			g.rows += n
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return rep, err
		}
		rows.Close()
	}
	rep.Distinct = len(groups)
	for key, g := range groups {
		rep.Rows += g.rows
		switch {
		case listed[key]:
			rep.Listed++
			continue
		case removed[key]:
			rep.Removed++
			continue
		}
		best, bn := "", -1
		for sp, n := range g.spell {
			if n > bn || (n == bn && sp < best) {
				best, bn = sp, n
			}
		}
		if _, err := validate.VocabName(kind, best); err != nil {
			rep.Invalid++
			continue
		}
		rep.ToAdd = append(rep.ToAdd, VocabCandidate{Name: best, Rows: g.rows})
	}
	sort.Slice(rep.ToAdd, func(i, j int) bool {
		if rep.ToAdd[i].Rows != rep.ToAdd[j].Rows {
			return rep.ToAdd[i].Rows > rep.ToAdd[j].Rows
		}
		return rep.ToAdd[i].Name < rep.ToAdd[j].Name
	})
	return rep, nil
}

// VocabBackfillChange is what one BackfillApply did: one undo step.
type VocabBackfillChange struct {
	Kind  string
	Added []model.VocabEntry
}

// BackfillApply adds every value the plan lists, in one transaction. Running it again adds nothing.
func (r *VocabRepo) BackfillApply(ctx context.Context, kind string) (VocabBackfillReport, VocabBackfillChange, error) {
	rep, err := r.BackfillPlan(ctx, kind)
	if err != nil {
		return rep, VocabBackfillChange{}, err
	}
	ch := VocabBackfillChange{Kind: kind}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return rep, ch, err
	}
	defer tx.Rollback()
	for _, c := range rep.ToAdd {
		e, err := addIn(ctx, tx, kind, c.Name)
		if err != nil {
			return rep, VocabBackfillChange{Kind: kind}, err
		}
		ch.Added = append(ch.Added, *e)
	}
	return rep, ch, tx.Commit()
}

// UndoBackfill takes the added values off the list again (they are deleted, the backfill created
// them). The records are untouched in both directions.
func (r *VocabRepo) UndoBackfill(ctx context.Context, ch VocabBackfillChange) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, e := range ch.Added {
		if _, err := tx.ExecContext(ctx, "DELETE FROM vocabulary WHERE id = ? AND uuid = ?", e.ID, e.UUID); err != nil {
			return fmt.Errorf("undo backfill: %w", err)
		}
	}
	return tx.Commit()
}

// RedoBackfill puts the values back with the same ids and positions.
func (r *VocabRepo) RedoBackfill(ctx context.Context, ch VocabBackfillChange) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, e := range ch.Added {
		if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO vocabulary (id, uuid, kind, name, position) VALUES (?, ?, ?, ?, ?)", e.ID, e.UUID, e.Kind, e.Name, e.Position); err != nil {
			return fmt.Errorf("redo backfill: %w", err)
		}
	}
	return tx.Commit()
}

// Discard deletes a value that was just added, for undo. Records that hold the text are untouched.
func (r *VocabRepo) Discard(ctx context.Context, e model.VocabEntry) error {
	if _, err := r.db.ExecContext(ctx, "DELETE FROM vocabulary WHERE id = ? AND uuid = ?", e.ID, e.UUID); err != nil {
		return fmt.Errorf("undo add value: %w", err)
	}
	return nil
}

// Restore puts a discarded value back with its id and position, for redo. A value of the same name
// that exists meanwhile is left alone.
func (r *VocabRepo) Restore(ctx context.Context, e model.VocabEntry) error {
	if _, err := r.db.ExecContext(ctx, "INSERT OR IGNORE INTO vocabulary (id, uuid, kind, name, position) VALUES (?, ?, ?, ?, ?)", e.ID, e.UUID, e.Kind, e.Name, e.Position); err != nil {
		return fmt.Errorf("redo add value: %w", err)
	}
	return nil
}
