package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// entityTables maps tagging entity types to the table that holds them.
var entityTables = map[string]string{
	"person": "people", "organization": "organizations",
	"deal": "deals", "interaction": "interactions", "task": "tasks",
}

// touchEntity bumps updated_at on a tagged entity. Table names come from entityTables only.
func (r *TagRepo) touchEntity(ctx context.Context, entityType string, entityID int64) error {
	table, ok := entityTables[entityType]
	if !ok {
		return fmt.Errorf("invalid entity type %q: %w", entityType, model.ErrValidation)
	}
	if _, err := r.db.ExecContext(ctx,
		fmt.Sprintf("UPDATE %s SET updated_at = datetime('now') WHERE id = ?", table), entityID); err != nil {
		return fmt.Errorf("touch %s %d: %w", entityType, entityID, err)
	}
	return nil
}

// TagRepo handles tag database operations.
type TagRepo struct {
	db *sql.DB
}

// NewTagRepo creates a new TagRepo.
func NewTagRepo(db *sql.DB) *TagRepo {
	return &TagRepo{db: db}
}

// FindAll lists the tags that at least one object carries. A tag is a name on objects with no record
// lifecycle of its own (owner decision 2026-10-27), so a tag with no live tagging is not listed.
func (r *TagRepo) FindAll(ctx context.Context) ([]*model.Tag, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT t.id, t.name FROM tags t
		WHERE EXISTS (SELECT 1 FROM taggings g WHERE g.tag_id = t.id AND g.deleted_at IS NULL) ORDER BY t.name`)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()

	var tags []*model.Tag
	for rows.Next() {
		var t model.Tag
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, &t)
	}
	return tags, rows.Err()
}

// FindOrCreate returns a tag by name, creating it if it doesn't exist.
// A new name is checked as a hierarchy path (nested tags, internal/validate): work/clients is a child of work.
func (r *TagRepo) FindOrCreate(ctx context.Context, name string) (*model.Tag, error) {
	var t model.Tag
	err := r.db.QueryRowContext(ctx, "SELECT id, name FROM tags WHERE name = ?", name).Scan(&t.ID, &t.Name)
	if err == nil {
		return &t, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("find tag: %w", err)
	}
	if name, err = cleanTagName(name); err != nil {
		return nil, err
	}
	if err := r.db.QueryRowContext(ctx, "SELECT id, name FROM tags WHERE name = ?", name).Scan(&t.ID, &t.Name); err == nil {
		return &t, nil
	}

	result, err := r.db.ExecContext(ctx,
		"INSERT INTO tags (name, uuid, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))",
		name, uuid.New().String())
	if err != nil {
		return nil, fmt.Errorf("create tag: %w", err)
	}
	id, _ := result.LastInsertId()
	return &model.Tag{ID: id, Name: name}, nil
}

// Apply adds a tag to an entity. No-op if already applied.
func (r *TagRepo) Apply(ctx context.Context, entityType string, entityID int64, tagName string) error {
	if !model.ValidTaggable(entityType) {
		return fmt.Errorf("invalid entity type %q: %w", entityType, model.ErrValidation)
	}

	// Verify entity exists. Table name is safe — entityType is validated above.
	table := entityTables[entityType]
	var exists int
	err := r.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT 1 FROM %s WHERE id = ? AND archived = 0", table), entityID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("%s %d: %w", entityType, entityID, model.ErrNotFound)
	}

	tag, err := r.FindOrCreate(ctx, tagName)
	if err != nil {
		return err
	}

	changed, err := r.tagTx(ctx, tag.ID, entityType, entityID)
	if err != nil {
		return err
	}
	if changed {
		return r.touchEntity(ctx, entityType, entityID)
	}
	return nil
}

// tagTx puts the tag on the entity: a live tagging stays, the newest soft deleted one is revived
// with its old row (same uuid), else a row is inserted. It says whether anything changed.
func (r *TagRepo) tagTx(ctx context.Context, tagID int64, entityType string, entityID int64) (bool, error) {
	var live int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM taggings WHERE tag_id = ? AND entity_type = ? AND entity_id = ? AND deleted_at IS NULL",
		tagID, entityType, entityID).Scan(&live); err != nil {
		return false, fmt.Errorf("find tagging: %w", err)
	}
	if live > 0 {
		return false, nil
	}
	res, err := r.db.ExecContext(ctx, `UPDATE taggings SET deleted_at = NULL, updated_at = datetime('now') WHERE id = (
			SELECT id FROM taggings WHERE tag_id = ? AND entity_type = ? AND entity_id = ? AND deleted_at IS NOT NULL
			ORDER BY deleted_at DESC, id DESC LIMIT 1)`, tagID, entityType, entityID)
	if err != nil {
		return false, fmt.Errorf("revive tagging: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return true, nil
	}
	if _, err := r.db.ExecContext(ctx,
		"INSERT INTO taggings (uuid, tag_id, entity_type, entity_id) VALUES (?, ?, ?, ?)",
		uuid.New().String(), tagID, entityType, entityID); err != nil {
		return false, fmt.Errorf("apply tag: %w", err)
	}
	return true, nil
}

// Remove removes a tag from an entity.
func (r *TagRepo) Remove(ctx context.Context, entityType string, entityID int64, tagName string) error {
	if !model.ValidTaggable(entityType) {
		return fmt.Errorf("invalid entity type %q: %w", entityType, model.ErrValidation)
	}

	result, err := r.db.ExecContext(ctx,
		`UPDATE taggings SET deleted_at = datetime('now'), updated_at = datetime('now')
		 WHERE tag_id = (SELECT id FROM tags WHERE name = ?) AND entity_type = ? AND entity_id = ? AND deleted_at IS NULL`,
		tagName, entityType, entityID)
	if err != nil {
		return fmt.Errorf("remove tag: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("tag %q on %s %d: %w", tagName, entityType, entityID, model.ErrNotFound)
	}
	return r.touchEntity(ctx, entityType, entityID)
}

// GetForEntity returns all tags for an entity.
func (r *TagRepo) GetForEntity(ctx context.Context, entityType string, entityID int64) ([]*model.Tag, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT t.id, t.name FROM tags t JOIN taggings tg ON tg.tag_id = t.id
		 WHERE tg.entity_type = ? AND tg.entity_id = ? AND tg.deleted_at IS NULL ORDER BY t.name`,
		entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("get tags for %s %d: %w", entityType, entityID, err)
	}
	defer rows.Close()

	var tags []*model.Tag
	for rows.Next() {
		var t model.Tag
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, &t)
	}
	return tags, rows.Err()
}

// GetEntities returns entity IDs with a given tag.
func (r *TagRepo) GetEntities(ctx context.Context, tagName string, entityType string) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT tg.entity_id FROM taggings tg JOIN tags t ON t.id = tg.tag_id
		 WHERE t.name = ? AND tg.entity_type = ? AND tg.deleted_at IS NULL`,
		tagName, entityType)
	if err != nil {
		return nil, fmt.Errorf("get entities for tag %q: %w", tagName, err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan entity id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Delete untags everything that carries the tag. See DeleteTag.
func (r *TagRepo) Delete(ctx context.Context, tagName string) error {
	_, err := r.DeleteTag(ctx, tagName)
	return err
}
