package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/validate"
)

// Removal of one or more values of a user-managed list as ONE reversible step (patterns/list-editor-standard).
// The TUI list editor removes the highlighted or selected items together, decides where the records
// that use them go (to another item, blank, kept, or unlinked) and undoes the whole removal with
// one ctrl+z. Each list kind has its own RemoveMany and UndoRemoval here, written over the same
// snapshot idea: before the change the rows it will touch are read, so undo puts every one of them
// back exactly, including a deal's close date.

// cellSnap is one record row before a removal touched it.
type cellSnap struct {
	ID       int64
	Value    sql.NullString
	ClosedAt sql.NullString
}

// snapshotCells reads the rows of a table whose column holds name (any case), for undo.
func snapshotCells(ctx context.Context, q vocabQuerier, table, column, name string, withClosed bool) ([]cellSnap, error) {
	cols := "id, " + column
	if withClosed {
		cols += ", closed_at"
	}
	rows, err := q.QueryContext(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE %s = ? COLLATE NOCASE", cols, table, column), name)
	if err != nil {
		return nil, fmt.Errorf("read %s.%s: %w", table, column, err)
	}
	defer rows.Close()
	var out []cellSnap
	for rows.Next() {
		var s cellSnap
		if withClosed {
			err = rows.Scan(&s.ID, &s.Value, &s.ClosedAt)
		} else {
			err = rows.Scan(&s.ID, &s.Value)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// restoreCells puts the snapshotted rows back.
func restoreCells(ctx context.Context, q vocabQuerier, table, column string, snaps []cellSnap, withClosed bool) error {
	for _, s := range snaps {
		var err error
		if withClosed {
			_, err = q.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s = ?, closed_at = ?, updated_at = datetime('now') WHERE id = ?", table, column), s.Value, s.ClosedAt, s.ID)
		} else {
			_, err = q.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s = ?, updated_at = datetime('now') WHERE id = ?", table, column), s.Value, s.ID)
		}
		if err != nil {
			return fmt.Errorf("restore %s.%s: %w", table, column, err)
		}
	}
	return nil
}

// --- vocabularies (deal stages, deal roles) ---

// VocabRemoveAction says where the records of the removed values go. With neither field set, a value
// that records use is E_VOCAB_IN_USE.
type VocabRemoveAction struct {
	// ReplaceWith is the listed value the records move to.
	ReplaceWith string
	// Blank clears the field on the records (a vocabulary that is not Strict).
	Blank bool
}

type vocabRowSnap struct {
	Table, Column string
	Closed        bool
	Cells         []cellSnap
}

// VocabRemoval is a finished removal: what to put back to undo it, and what it did.
type VocabRemoval struct {
	Kind    string
	Names   []string
	Action  VocabRemoveAction
	Entries []model.VocabEntry
	rows    []vocabRowSnap
	// Moved is how many records were given the replacement, Blanked how many were cleared.
	Moved, Blanked int
	ReplacedBy     string
}

// stageRulesDropping checks validate.StageRules on the live stages left after dropping the given ids.
func stageRulesDropping(ctx context.Context, q vocabQuerier, kind string, drop map[int64]bool) error {
	rows, err := q.QueryContext(ctx, "SELECT id, name, outcome FROM vocabulary WHERE kind = ? AND archived = 0", kind)
	if err != nil {
		return fmt.Errorf("read stages: %w", err)
	}
	defer rows.Close()
	var after []validate.StageState
	for rows.Next() {
		var id int64
		var st validate.StageState
		if err := rows.Scan(&id, &st.Name, &st.Outcome); err != nil {
			return err
		}
		if !drop[id] {
			after = append(after, st)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return validate.StageRules(after)
}

// RemoveMany takes several values off the list in one transaction and moves, clears or leaves the
// records that use them. A strict vocabulary (deal stages) refuses Blank and keeps the stage rules
// true for the list that remains. The returned removal undoes with UndoRemoval and repeats with
// RedoRemoval.
func (r *VocabRepo) RemoveMany(ctx context.Context, kind string, names []string, act VocabRemoveAction) (*VocabRemoval, error) {
	vs, err := vocabSpec(kind)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("nothing to remove: %w", model.ErrValidation)
	}
	if act.ReplaceWith != "" && act.Blank {
		return nil, fmt.Errorf("use a replacement or blank, not both: %w", model.ErrValidation)
	}
	if act.Blank && vs.Strict {
		return nil, fmt.Errorf("a %s cannot be blank, pick another %s: %w", vs.Noun, vs.Noun, model.ErrValidation)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rem := &VocabRemoval{Kind: kind, Action: act}
	drop := map[int64]bool{}
	var cur []*model.VocabEntry
	for _, n := range names {
		e, err := findVocab(ctx, tx, kind, n)
		if err != nil || e.Removed {
			return nil, vocabNotFound(vs, n)
		}
		if drop[e.ID] {
			continue
		}
		drop[e.ID] = true
		cur = append(cur, e)
		rem.Names = append(rem.Names, e.Name)
		rem.Entries = append(rem.Entries, *e)
	}
	if vs.Outcomes {
		if err := stageRulesDropping(ctx, tx, kind, drop); err != nil {
			return nil, err
		}
	}
	var rep *model.VocabEntry
	if act.ReplaceWith != "" {
		rep, err = findVocab(ctx, tx, kind, act.ReplaceWith)
		if err != nil || rep.Removed {
			return nil, vocabNotFound(vs, act.ReplaceWith)
		}
		if drop[rep.ID] {
			return nil, fmt.Errorf("the replacement must be a %s that stays: %w", vs.Noun, model.ErrValidation)
		}
		rem.ReplacedBy = rep.Name
	}
	for _, e := range cur {
		n, err := usage(ctx, tx, vs, e.Name)
		if err != nil {
			return nil, err
		}
		if n > 0 && rep == nil && !act.Blank {
			return nil, validate.VocabInUse(kind, e.Name, n)
		}
		for _, u := range vs.Usages {
			closed := kind == model.DealStageKind
			snaps, err := snapshotCells(ctx, tx, u.Table, u.Column, e.Name, closed)
			if err != nil {
				return nil, err
			}
			if len(snaps) == 0 {
				continue
			}
			rem.rows = append(rem.rows, vocabRowSnap{Table: u.Table, Column: u.Column, Closed: closed, Cells: snaps})
			switch {
			case rep != nil:
				extra := ""
				if closed {
					extra = closedAtOnMove(e.Outcome, rep.Outcome)
				}
				out, err := tx.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s = ?, updated_at = datetime('now')%s WHERE %s = ? COLLATE NOCASE", u.Table, u.Column, extra, u.Column), rep.Name, e.Name)
				if err != nil {
					return nil, fmt.Errorf("move %s.%s: %w", u.Table, u.Column, err)
				}
				c, _ := out.RowsAffected()
				rem.Moved += int(c)
			case act.Blank:
				out, err := tx.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s = NULL, updated_at = datetime('now') WHERE %s = ? COLLATE NOCASE", u.Table, u.Column, u.Column), e.Name)
				if err != nil {
					return nil, fmt.Errorf("clear %s.%s: %w", u.Table, u.Column, err)
				}
				c, _ := out.RowsAffected()
				rem.Blanked += int(c)
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE vocabulary SET archived = 1, updated_at = datetime('now') WHERE id = ?", e.ID); err != nil {
			return nil, fmt.Errorf("remove value: %w", err)
		}
	}
	return rem, tx.Commit()
}

// UndoRemoval puts the values back on the list and every record as it was.
func (r *VocabRepo) UndoRemoval(ctx context.Context, rem *VocabRemoval) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, e := range rem.Entries {
		if _, err := tx.ExecContext(ctx, "UPDATE vocabulary SET archived = 0, updated_at = datetime('now') WHERE id = ?", e.ID); err != nil {
			return fmt.Errorf("undo remove: %w", err)
		}
	}
	for _, s := range rem.rows {
		if err := restoreCells(ctx, tx, s.Table, s.Column, s.Cells, s.Closed); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RedoRemoval repeats an undone removal. The rows it touches are read again, so the removal stays
// undoable.
func (r *VocabRepo) RedoRemoval(ctx context.Context, rem *VocabRemoval) error {
	again, err := r.RemoveMany(ctx, rem.Kind, rem.Names, rem.Action)
	if err != nil {
		return err
	}
	*rem = *again
	return nil
}

// --- note types ---

// NoteTypeRemoval is a finished removal of note types.
type NoteTypeRemoval struct {
	Names       []string
	ReplaceWith string
	types       []model.NoteType
	cells       []cellSnap
	Moved, Kept int
}

// protectedNoteType is the type the program relies on (the default of a new note).
const protectedNoteType = "note"

// IsProtectedNoteType reports whether a type cannot be renamed or removed.
func IsProtectedNoteType(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), protectedNoteType)
}

// Counts returns how many live notes use each type, by lower-case name.
func (r *NoteTypeRepo) Counts(ctx context.Context) (map[string]int, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT lower(type), COUNT(*) FROM interactions WHERE archived = 0 GROUP BY lower(type)")
	if err != nil {
		return nil, fmt.Errorf("count note types: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var n string
		var c int
		if err := rows.Scan(&n, &c); err != nil {
			return nil, err
		}
		out[n] = c
	}
	return out, rows.Err()
}

// Rename changes the name of a type and rewrites the notes that use it. The default type
// "note" keeps its name.
func (r *NoteTypeRepo) Rename(ctx context.Context, from, to string) (RenameResult, error) {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if IsProtectedNoteType(from) {
		return RenameResult{}, fmt.Errorf("note is the default type and keeps its name: %w", model.ErrConflict)
	}
	if !noteTypeNameRE.MatchString(to) {
		return RenameResult{}, fmt.Errorf("invalid note type name %q (letters, digits, - and _ only): %w", to, model.ErrValidation)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return RenameResult{}, err
	}
	defer tx.Rollback()
	cur, err := scanNoteType(tx.QueryRowContext(ctx, "SELECT "+noteTypeColumns+" FROM note_types WHERE name = ? AND archived = 0", from))
	if err != nil {
		return RenameResult{}, fmt.Errorf("note type %q: %w", from, model.ErrNotFound)
	}
	var other int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM note_types WHERE name = ? AND id <> ?", to, cur.ID).Scan(&other)
	if err == nil {
		return RenameResult{}, fmt.Errorf("a note type named %q already exists (add it to bring an archived one back): %w", to, model.ErrConflict)
	}
	res := RenameResult{Old: cur.Name, New: to}
	if _, err := tx.ExecContext(ctx, "UPDATE note_types SET name = ?, updated_at = datetime('now') WHERE id = ?", to, cur.ID); err != nil {
		return RenameResult{}, fmt.Errorf("rename note type: %w", err)
	}
	out, err := tx.ExecContext(ctx, "UPDATE interactions SET type = ?, updated_at = datetime('now') WHERE type = ? COLLATE NOCASE", to, cur.Name)
	if err != nil {
		return RenameResult{}, fmt.Errorf("rewrite notes: %w", err)
	}
	n, _ := out.RowsAffected()
	res.Rows = int(n)
	return res, tx.Commit()
}

// SetCountsAsContact sets whether a note of the type counts as contact.
func (r *NoteTypeRepo) SetCountsAsContact(ctx context.Context, name string, counts bool) error {
	res, err := r.db.ExecContext(ctx, "UPDATE note_types SET counts_as_contact = ?, updated_at = datetime('now') WHERE name = ? AND archived = 0", counts, strings.TrimSpace(name))
	if err != nil {
		return fmt.Errorf("set counts as contact: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("note type %q: %w", name, model.ErrNotFound)
	}
	return nil
}

// Reorder puts the named active types first in the given order, the others follow in their order.
func (r *NoteTypeRepo) Reorder(ctx context.Context, names []string) error {
	list, err := r.List(ctx, false)
	if err != nil {
		return err
	}
	by := map[string]*model.NoteType{}
	for _, t := range list {
		by[strings.ToLower(t.Name)] = t
	}
	var order []*model.NoteType
	seen := map[int64]bool{}
	for _, n := range names {
		t, ok := by[strings.ToLower(strings.TrimSpace(n))]
		if !ok {
			return fmt.Errorf("note type %q: %w", n, model.ErrNotFound)
		}
		if !seen[t.ID] {
			seen[t.ID] = true
			order = append(order, t)
		}
	}
	for _, t := range list {
		if !seen[t.ID] {
			order = append(order, t)
		}
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, t := range order {
		if t.Position == i+1 {
			continue
		}
		if _, err := tx.ExecContext(ctx, "UPDATE note_types SET position = ?, updated_at = datetime('now') WHERE id = ?", i+1, t.ID); err != nil {
			return fmt.Errorf("reorder note types: %w", err)
		}
	}
	return tx.Commit()
}

// RemoveMany archives several types in one transaction. The notes that use them move to replaceWith
// (an active type that stays) or, with replaceWith empty, keep the old type name as before (an
// archived type, "old notes keep it"). The default type "note" stays.
func (r *NoteTypeRepo) RemoveMany(ctx context.Context, names []string, replaceWith string) (*NoteTypeRemoval, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("nothing to remove: %w", model.ErrValidation)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rem := &NoteTypeRemoval{ReplaceWith: replaceWith}
	drop := map[int64]bool{}
	var cur []*model.NoteType
	for _, n := range names {
		if IsProtectedNoteType(n) {
			return nil, fmt.Errorf("note is the default type and stays: %w", model.ErrConflict)
		}
		t, err := scanNoteType(tx.QueryRowContext(ctx, "SELECT "+noteTypeColumns+" FROM note_types WHERE name = ? AND archived = 0", strings.TrimSpace(n)))
		if err != nil {
			return nil, fmt.Errorf("note type %q: %w", n, model.ErrNotFound)
		}
		if drop[t.ID] {
			continue
		}
		drop[t.ID] = true
		cur = append(cur, t)
		rem.Names = append(rem.Names, t.Name)
		rem.types = append(rem.types, *t)
	}
	var rep *model.NoteType
	if replaceWith != "" {
		rep, err = scanNoteType(tx.QueryRowContext(ctx, "SELECT "+noteTypeColumns+" FROM note_types WHERE name = ? AND archived = 0", strings.TrimSpace(replaceWith)))
		if err != nil || drop[rep.ID] {
			return nil, fmt.Errorf("the replacement must be an active note type that stays: %w", model.ErrValidation)
		}
		rem.ReplaceWith = rep.Name
	}
	for _, t := range cur {
		snaps, err := snapshotCells(ctx, tx, "interactions", "type", t.Name, false)
		if err != nil {
			return nil, err
		}
		if rep != nil && len(snaps) > 0 {
			rem.cells = append(rem.cells, snaps...)
			out, err := tx.ExecContext(ctx, "UPDATE interactions SET type = ?, updated_at = datetime('now') WHERE type = ? COLLATE NOCASE", rep.Name, t.Name)
			if err != nil {
				return nil, fmt.Errorf("move notes: %w", err)
			}
			c, _ := out.RowsAffected()
			rem.Moved += int(c)
		} else {
			rem.Kept += len(snaps)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE note_types SET archived = 1, updated_at = datetime('now') WHERE id = ?", t.ID); err != nil {
			return nil, fmt.Errorf("archive note type: %w", err)
		}
	}
	return rem, tx.Commit()
}

// UndoRemoval brings the types back and every note as it was.
func (r *NoteTypeRepo) UndoRemoval(ctx context.Context, rem *NoteTypeRemoval) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range rem.types {
		if _, err := tx.ExecContext(ctx, "UPDATE note_types SET archived = 0, updated_at = datetime('now') WHERE id = ?", t.ID); err != nil {
			return fmt.Errorf("undo archive: %w", err)
		}
	}
	if err := restoreCells(ctx, tx, "interactions", "type", rem.cells, false); err != nil {
		return err
	}
	return tx.Commit()
}

// RedoRemoval repeats an undone removal.
func (r *NoteTypeRepo) RedoRemoval(ctx context.Context, rem *NoteTypeRemoval) error {
	again, err := r.RemoveMany(ctx, rem.Names, rem.ReplaceWith)
	if err != nil {
		return err
	}
	*rem = *again
	return nil
}
