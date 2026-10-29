package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// Editing relation kinds from the list editor (patterns/list-editor-standard): rename, change the
// inverse name, reorder, and remove with a decision for the links that use the kind.

func kindTable(objectType string) string {
	if objectType == "organization" {
		return "org_relation"
	}
	return "person_relation"
}

// Counts returns how many live links use each kind of an object type, by kind id.
func (r *KindRepo) Counts(ctx context.Context, objectType string) (map[int64]int, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT kind_id, COUNT(*) FROM "+kindTable(objectType)+" WHERE deleted_at IS NULL GROUP BY kind_id")
	if err != nil {
		return nil, fmt.Errorf("count links: %w", err)
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// KindPatch changes a user-defined kind. A nil field is left alone. An empty Inverse makes the
// kind mutual.
type KindPatch struct {
	Name    *string
	Inverse *string
}

// Update renames a user-defined kind or changes its inverse name. Built-in kinds stay as they are.
// A kind that links use keeps its direction: it cannot change between mutual and directional.
func (r *KindRepo) Update(ctx context.Context, objectType, name string, p KindPatch) (*model.RelationKind, error) {
	k, _, err := r.Resolve(ctx, objectType, name)
	if err != nil {
		return nil, err
	}
	if k.Builtin {
		return nil, fmt.Errorf("%q is a built-in kind and cannot be changed: %w", k.Name, model.ErrConflict)
	}
	newName, newInv := k.Name, k.InverseName
	if p.Name != nil {
		newName = strings.TrimSpace(*p.Name)
		if newName == "" {
			return nil, fmt.Errorf("a kind needs a name: %w", model.ErrValidation)
		}
		if k.Symmetric {
			newInv = newName
		}
	}
	if p.Inverse != nil {
		inv := strings.TrimSpace(*p.Inverse)
		if inv == "" || strings.EqualFold(inv, newName) {
			inv = newName
		}
		newInv = inv
	}
	symmetric := strings.EqualFold(newInv, newName)
	if symmetric {
		newInv = newName
	}
	if symmetric != k.Symmetric {
		counts, err := r.Counts(ctx, objectType)
		if err != nil {
			return nil, err
		}
		if n := counts[k.ID]; n > 0 {
			return nil, fmt.Errorf("%d link(s) use %q, so it cannot change between mutual and directional. Move or unlink them first: %w", n, k.Name, model.ErrConflict)
		}
	}
	var clash int
	err = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM relation_kind WHERE object_type = ? AND deleted_at IS NULL AND id <> ?
		AND (name IN (?, ?) OR inverse_name IN (?, ?))`, objectType, k.ID, newName, newInv, newName, newInv).Scan(&clash)
	if err != nil {
		return nil, fmt.Errorf("check kind: %w", err)
	}
	if clash > 0 {
		return nil, fmt.Errorf("a %s kind named %q or %q already exists: %w", objectType, newName, newInv, model.ErrConflict)
	}
	phrase := k.Phrase && !symmetric
	if _, err := r.db.ExecContext(ctx, "UPDATE relation_kind SET name = ?, inverse_name = ?, symmetric = ?, phrase = ?, updated_at = datetime('now') WHERE id = ?",
		newName, newInv, symmetric, phrase, k.ID); err != nil {
		return nil, fmt.Errorf("update kind: %w", err)
	}
	return r.FindByID(ctx, k.ID)
}

// Reorder puts the named kinds of an object type first in the given order, the others follow.
func (r *KindRepo) Reorder(ctx context.Context, objectType string, names []string) error {
	list, err := r.List(ctx, objectType)
	if err != nil {
		return err
	}
	by := map[string]*model.RelationKind{}
	for _, k := range list {
		by[strings.ToLower(k.Name)] = k
	}
	var order []*model.RelationKind
	seen := map[int64]bool{}
	for _, n := range names {
		k, ok := by[strings.ToLower(strings.TrimSpace(n))]
		if !ok {
			return fmt.Errorf("%s kind %q: %w", objectType, n, model.ErrNotFound)
		}
		if !seen[k.ID] {
			seen[k.ID] = true
			order = append(order, k)
		}
	}
	for _, k := range list {
		if !seen[k.ID] {
			order = append(order, k)
		}
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, k := range order {
		if k.Position == i+1 {
			continue
		}
		if _, err := tx.ExecContext(ctx, "UPDATE relation_kind SET position = ?, updated_at = datetime('now') WHERE id = ?", i+1, k.ID); err != nil {
			return fmt.Errorf("reorder kinds: %w", err)
		}
	}
	return tx.Commit()
}

// KindRemoveAction says what happens to the links of the removed kinds. With neither field set a
// kind that links use is a conflict.
type KindRemoveAction struct {
	// MoveTo is the kind the links move to.
	MoveTo string
	// Unlink deletes the links (they can be restored by undo).
	Unlink bool
}

type linkSnap struct {
	ID, A, B, Kind int64
	Deleted        sql.NullString
}

// KindRemoval is a finished removal of relation kinds.
type KindRemoval struct {
	ObjectType string
	Names      []string
	Action     KindRemoveAction
	kinds      []int64
	links      []linkSnap
	Moved      int
	Unlinked   int
}

// RemoveMany soft-deletes several user-defined kinds in one transaction and moves or unlinks their
// links. Built-in kinds stay. Moving links onto a kind that already links the same two records is a
// conflict and nothing changes.
func (r *KindRepo) RemoveMany(ctx context.Context, objectType string, names []string, act KindRemoveAction) (*KindRemoval, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("nothing to remove: %w", model.ErrValidation)
	}
	if act.MoveTo != "" && act.Unlink {
		return nil, fmt.Errorf("use a target kind or unlink, not both: %w", model.ErrValidation)
	}
	table := kindTable(objectType)
	rem := &KindRemoval{ObjectType: objectType, Action: act}
	drop := map[int64]bool{}
	var cur []*model.RelationKind
	for _, n := range names {
		k, _, err := r.Resolve(ctx, objectType, n)
		if err != nil {
			return nil, err
		}
		if k.Builtin {
			return nil, fmt.Errorf("%q is a built-in kind and cannot be removed: %w", k.Name, model.ErrConflict)
		}
		if drop[k.ID] {
			continue
		}
		drop[k.ID] = true
		cur = append(cur, k)
		rem.Names = append(rem.Names, k.Name)
		rem.kinds = append(rem.kinds, k.ID)
	}
	var target *model.RelationKind
	if act.MoveTo != "" {
		var err error
		target, _, err = r.Resolve(ctx, objectType, act.MoveTo)
		if err != nil {
			return nil, err
		}
		if drop[target.ID] {
			return nil, fmt.Errorf("the target kind must be one that stays: %w", model.ErrValidation)
		}
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, k := range cur {
		rows, err := tx.QueryContext(ctx, "SELECT id, a_id, b_id, kind_id FROM "+table+" WHERE kind_id = ? AND deleted_at IS NULL", k.ID)
		if err != nil {
			return nil, fmt.Errorf("read links: %w", err)
		}
		var links []linkSnap
		for rows.Next() {
			var s linkSnap
			if err := rows.Scan(&s.ID, &s.A, &s.B, &s.Kind); err != nil {
				rows.Close()
				return nil, err
			}
			links = append(links, s)
		}
		rows.Close()
		if len(links) > 0 && target == nil && !act.Unlink {
			return nil, fmt.Errorf("kind %q is used by %d link(s), move them to another kind or unlink them: %w", k.Name, len(links), model.ErrConflict)
		}
		for _, s := range links {
			rem.links = append(rem.links, s)
			if act.Unlink {
				if _, err := tx.ExecContext(ctx, "UPDATE "+table+" SET deleted_at = datetime('now'), updated_at = datetime('now') WHERE id = ?", s.ID); err != nil {
					return nil, fmt.Errorf("unlink: %w", err)
				}
				rem.Unlinked++
				continue
			}
			a, b := s.A, s.B
			if target.Symmetric && a > b {
				a, b = b, a
			}
			var dup int
			if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE a_id = ? AND b_id = ? AND kind_id = ? AND deleted_at IS NULL", a, b, target.ID).Scan(&dup); err != nil {
				return nil, err
			}
			if dup > 0 {
				return nil, fmt.Errorf("two records are already linked as %q, so %q cannot move there. Unlink them first: %w", target.Name, k.Name, model.ErrConflict)
			}
			if _, err := tx.ExecContext(ctx, "UPDATE "+table+" SET a_id = ?, b_id = ?, kind_id = ?, updated_at = datetime('now') WHERE id = ?", a, b, target.ID, s.ID); err != nil {
				return nil, fmt.Errorf("move link: %w", err)
			}
			rem.Moved++
		}
		if _, err := tx.ExecContext(ctx, "UPDATE relation_kind SET deleted_at = datetime('now'), updated_at = datetime('now') WHERE id = ?", k.ID); err != nil {
			return nil, fmt.Errorf("remove kind: %w", err)
		}
	}
	return rem, tx.Commit()
}

// UndoRemoval brings the kinds and every link back as it was.
func (r *KindRepo) UndoRemoval(ctx context.Context, rem *KindRemoval) error {
	table := kindTable(rem.ObjectType)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range rem.kinds {
		if _, err := tx.ExecContext(ctx, "UPDATE relation_kind SET deleted_at = NULL, updated_at = datetime('now') WHERE id = ?", id); err != nil {
			return fmt.Errorf("undo remove kind: %w", err)
		}
	}
	for _, s := range rem.links {
		if _, err := tx.ExecContext(ctx, "UPDATE "+table+" SET a_id = ?, b_id = ?, kind_id = ?, deleted_at = NULL, updated_at = datetime('now') WHERE id = ?", s.A, s.B, s.Kind, s.ID); err != nil {
			return fmt.Errorf("undo links: %w", err)
		}
	}
	return tx.Commit()
}

// RedoRemoval repeats an undone removal.
func (r *KindRepo) RedoRemoval(ctx context.Context, rem *KindRemoval) error {
	again, err := r.RemoveMany(ctx, rem.ObjectType, rem.Names, rem.Action)
	if err != nil {
		return err
	}
	*rem = *again
	return nil
}
