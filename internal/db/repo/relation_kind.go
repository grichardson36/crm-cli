package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// KindRepo manages relation kinds: the vocabulary of person and organization links.
type KindRepo struct {
	db *sql.DB
}

// NewKindRepo creates a KindRepo.
func NewKindRepo(db *sql.DB) *KindRepo { return &KindRepo{db: db} }

const kindColumns = "id, uuid, object_type, name, inverse_name, symmetric, builtin, created_at, updated_at"

func scanKind(row interface{ Scan(...any) error }) (*model.RelationKind, error) {
	var k model.RelationKind
	err := row.Scan(&k.ID, &k.UUID, &k.ObjectType, &k.Name, &k.InverseName, &k.Symmetric, &k.Builtin, &k.CreatedAt, &k.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &k, nil
}

// List returns live kinds. objectType "" lists both person and organization kinds.
func (r *KindRepo) List(ctx context.Context, objectType string) ([]*model.RelationKind, error) {
	q := "SELECT " + kindColumns + " FROM relation_kind WHERE deleted_at IS NULL"
	var args []any
	if objectType != "" {
		q += " AND object_type = ?"
		args = append(args, objectType)
	}
	q += " ORDER BY object_type DESC, builtin DESC, id"
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list relation kinds: %w", err)
	}
	defer rows.Close()
	var out []*model.RelationKind
	for rows.Next() {
		k, err := scanKind(rows)
		if err != nil {
			return nil, fmt.Errorf("scan relation kind: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// FindByID returns a live kind by id.
func (r *KindRepo) FindByID(ctx context.Context, id int64) (*model.RelationKind, error) {
	k, err := scanKind(r.db.QueryRowContext(ctx, "SELECT "+kindColumns+" FROM relation_kind WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("relation kind %d: %w", id, model.ErrNotFound)
	}
	return k, err
}

// Resolve finds a live kind by name or, for a directional kind, by its inverse name. swapped is
// true when the inverse name matched: the caller reverses the two ends.
func (r *KindRepo) Resolve(ctx context.Context, objectType, name string) (k *model.RelationKind, swapped bool, err error) {
	name = strings.TrimSpace(name)
	k, err = scanKind(r.db.QueryRowContext(ctx,
		"SELECT "+kindColumns+" FROM relation_kind WHERE object_type = ? AND name = ? AND deleted_at IS NULL", objectType, name))
	if err == nil {
		return k, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	k, err = scanKind(r.db.QueryRowContext(ctx,
		"SELECT "+kindColumns+" FROM relation_kind WHERE object_type = ? AND inverse_name = ? AND deleted_at IS NULL", objectType, name))
	if err == nil {
		return k, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		all, _ := r.List(ctx, objectType)
		names := make([]string, 0, len(all))
		for _, x := range all {
			names = append(names, x.Name)
		}
		return nil, false, fmt.Errorf("unknown %s link kind %q (known: %s): %w", objectType, name, strings.Join(names, ", "), model.ErrValidation)
	}
	return nil, false, err
}

// Add creates a kind. With no inverse name the kind is symmetric. Names must not clash with any
// name or inverse name of the same object type.
func (r *KindRepo) Add(ctx context.Context, objectType, name, inverse string) (*model.RelationKind, error) {
	if objectType != "person" && objectType != "organization" {
		return nil, fmt.Errorf("kinds are for person or organization links, not %q: %w", objectType, model.ErrValidation)
	}
	name, inverse = strings.TrimSpace(name), strings.TrimSpace(inverse)
	if name == "" {
		return nil, fmt.Errorf("a kind needs a name: %w", model.ErrValidation)
	}
	symmetric := inverse == "" || strings.EqualFold(inverse, name)
	if symmetric {
		inverse = name
	}
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM relation_kind WHERE object_type = ? AND deleted_at IS NULL
		AND (name IN (?, ?) OR inverse_name IN (?, ?))`, objectType, name, inverse, name, inverse).Scan(&n)
	if err != nil {
		return nil, fmt.Errorf("check kind: %w", err)
	}
	if n > 0 {
		return nil, fmt.Errorf("a %s kind named %q or %q already exists: %w", objectType, name, inverse, model.ErrConflict)
	}
	res, err := r.db.ExecContext(ctx, `INSERT INTO relation_kind (uuid, object_type, name, inverse_name, symmetric, builtin)
		VALUES (?, ?, ?, ?, ?, 0)`, uuid.New().String(), objectType, name, inverse, symmetric)
	if err != nil {
		return nil, fmt.Errorf("add kind: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.FindByID(ctx, id)
}

// Remove soft-deletes a user-defined kind that no live link uses. Built-in kinds stay.
func (r *KindRepo) Remove(ctx context.Context, objectType, name string) error {
	k, _, err := r.Resolve(ctx, objectType, name)
	if err != nil {
		return err
	}
	if k.Builtin {
		return fmt.Errorf("%q is a built-in kind and cannot be removed: %w", k.Name, model.ErrConflict)
	}
	table := "person_relation"
	if objectType == "organization" {
		table = "org_relation"
	}
	var n int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE kind_id = ? AND deleted_at IS NULL", k.ID).Scan(&n); err != nil {
		return fmt.Errorf("count links: %w", err)
	}
	if n > 0 {
		return fmt.Errorf("kind %q is used by %d link(s), unlink them first: %w", k.Name, n, model.ErrConflict)
	}
	_, err = r.db.ExecContext(ctx, "UPDATE relation_kind SET deleted_at = datetime('now'), updated_at = datetime('now') WHERE id = ?", k.ID)
	if err != nil {
		return fmt.Errorf("remove kind: %w", err)
	}
	return nil
}
