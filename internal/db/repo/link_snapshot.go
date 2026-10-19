package repo

import (
	"context"
	"fmt"
	"reflect"
	"sort"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// Snapshot returns the stored link rows that a change between a and b can touch: the rows of the
// pair, deleted ones included, and for relations with a primary flag the live rows of the A object.
// Undo and redo take a snapshot before and after a change and Apply the one they want back.
func (r *LinkRepo) Snapshot(ctx context.Context, a, b Ref) ([]LinkRecord, error) {
	rel, ok := model.RelationBetween(a.Kind, b.Kind)
	if !ok {
		return nil, fmt.Errorf("a %s cannot link to a %s: %w", a.Kind, b.Kind, model.ErrValidation)
	}
	ra, rb := a, b
	if rel.A != a.Kind {
		ra, rb = b, a
	}
	st := storeOf(rel)
	q := fmt.Sprintf("SELECT %s FROM %s WHERE ((%s = ? AND %s = ?) OR (%s = ? AND %s = ?)", st.selectList(""), st.table, st.aCol, st.bCol, st.aCol, st.bCol)
	args := []any{ra.ID, rb.ID}
	if rel.SameKind() {
		args = append(args, rb.ID, ra.ID)
	} else {
		args = append(args, ra.ID, rb.ID)
	}
	if rel.PrimaryPerA {
		q += fmt.Sprintf(" OR (%s = ? AND deleted_at IS NULL)", st.aCol)
		args = append(args, ra.ID)
	}
	q += ")" + st.fixedWhere("") + " ORDER BY id"
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("snapshot links: %w", err)
	}
	defer rows.Close()
	var out []LinkRecord
	for rows.Next() {
		rec, err := scanRecord(rel, rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rec)
	}
	return out, rows.Err()
}

// SameRecords reports whether two snapshots hold the same rows with the same values.
func SameRecords(a, b []LinkRecord) bool {
	if len(a) != len(b) {
		return false
	}
	norm := func(x []LinkRecord) []LinkRecord {
		y := append([]LinkRecord(nil), x...)
		sort.Slice(y, func(i, j int) bool { return y[i].ID < y[j].ID })
		return y
	}
	return reflect.DeepEqual(norm(a), norm(b))
}

// UndoSet is what puts the links back as they were: the rows of before, and a soft delete for every
// row after has and before did not.
func UndoSet(before, after []LinkRecord) []LinkRecord {
	had := map[int64]bool{}
	var out []LinkRecord
	for _, b := range before {
		had[b.ID] = true
		out = append(out, b)
	}
	for _, a := range after {
		if !had[a.ID] {
			a.Deleted = true
			out = append(out, a)
		}
	}
	return out
}

// Apply writes link rows exactly as the records say: attributes, primary flag and whether the row
// is deleted. It runs in one transaction. Rows that lose their primary flag are written first so the
// one that gains it does not collide.
func (r *LinkRepo) Apply(ctx context.Context, recs []LinkRecord) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin apply: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	ordered := append([]LinkRecord(nil), recs...)
	sort.SliceStable(ordered, func(i, j int) bool { return !ordered[i].Primary && ordered[j].Primary })
	for _, rec := range ordered {
		rel, ok := model.RelationNamed(rec.Relation)
		if !ok {
			return fmt.Errorf("unknown relation %q: %w", rec.Relation, model.ErrValidation)
		}
		st := storeOf(rel)
		if _, err := r.write(ctx, tx, rel, st, rec); err != nil {
			return err
		}
		val := "NULL"
		if rec.Deleted {
			val = "datetime('now')"
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET deleted_at = %s WHERE id = ?", st.table, val), rec.ID); err != nil {
			return fmt.Errorf("apply link: %w", err)
		}
		if err := r.touchBoth(ctx, tx, rel, rec.A, rec.B); err != nil {
			return err
		}
	}
	return tx.Commit()
}
