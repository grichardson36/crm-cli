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
	"deal": "deals", "interaction": "interactions", "event": "events",
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

// FindAll lists all tags.
func (r *TagRepo) FindAll(ctx context.Context) ([]*model.Tag, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, name FROM tags ORDER BY name")
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
func (r *TagRepo) FindOrCreate(ctx context.Context, name string) (*model.Tag, error) {
	var t model.Tag
	err := r.db.QueryRowContext(ctx, "SELECT id, name FROM tags WHERE name = ?", name).Scan(&t.ID, &t.Name)
	if err == nil {
		return &t, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("find tag: %w", err)
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
	if !model.ValidEntityType(entityType) {
		return fmt.Errorf("invalid entity type %q: %w", entityType, model.ErrValidation)
	}

	// Verify entity exists. Table name is safe — entityType is validated above.
	tableMap := map[string]string{
		"person": "people", "organization": "organizations",
		"deal": "deals", "interaction": "interactions", "event": "events",
	}
	table := tableMap[entityType]
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

	res, err := r.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO taggings (tag_id, entity_type, entity_id) VALUES (?, ?, ?)",
		tag.ID, entityType, entityID)
	if err != nil {
		return fmt.Errorf("apply tag: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return r.touchEntity(ctx, entityType, entityID)
	}
	return nil
}

// Remove removes a tag from an entity.
func (r *TagRepo) Remove(ctx context.Context, entityType string, entityID int64, tagName string) error {
	if !model.ValidEntityType(entityType) {
		return fmt.Errorf("invalid entity type %q: %w", entityType, model.ErrValidation)
	}

	result, err := r.db.ExecContext(ctx,
		`DELETE FROM taggings WHERE tag_id = (SELECT id FROM tags WHERE name = ?) AND entity_type = ? AND entity_id = ?`,
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
		 WHERE tg.entity_type = ? AND tg.entity_id = ? ORDER BY t.name`,
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
		 WHERE t.name = ? AND tg.entity_type = ?`,
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

// Delete removes a tag and all its taggings.
func (r *TagRepo) Delete(ctx context.Context, tagName string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Entities that lose the tag changed, so bump their updated_at before the taggings go.
	for entityType, table := range entityTables {
		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf(`UPDATE %s SET updated_at = datetime('now') WHERE id IN (
				SELECT tg.entity_id FROM taggings tg JOIN tags t ON t.id = tg.tag_id
				WHERE t.name = ? AND tg.entity_type = ?)`, table), tagName, entityType); err != nil {
			return fmt.Errorf("touch tagged %s: %w", entityType, err)
		}
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM tags WHERE name = ?", tagName)
	if err != nil {
		return fmt.Errorf("delete tag: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("tag %q: %w", tagName, model.ErrNotFound)
	}
	return tx.Commit()
}
