package repo

import (
	"context"
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// Deleting a tag (owner decision 2026-10-27, open discrepancy X26). A tag is a name on objects and
// has no record lifecycle, so there is no deleted_at on tags and no migration for one. Deleting a tag
// means untagging everything that carries it, as one step. The taggings are soft deleted: they keep
// their uuid and get deleted_at and updated_at, so a sync sees a tombstone for each. A tag with no
// live tagging drops out of tag lists. Undo and redo restore and remove those same rows.

// TagDeleteResult is what deleting a tag did: the taggings it soft deleted. Undo and redo use it.
type TagDeleteResult struct {
	Tag        string  `json:"tag"`
	TaggingIDs []int64 `json:"tagging_ids"`
	// Removed counts the objects that lost the tag, by entity type.
	Removed map[string]int `json:"removed"`
}

// Label is a short description for messages and the undo history.
func (r TagDeleteResult) Label() string { return "delete tag #" + r.Tag }

// DeleteTag untags every object that carries the tag, in one transaction. The result undoes and
// redoes the whole step. A tag no object carries is not found.
func (r *TagRepo) DeleteTag(ctx context.Context, tagName string) (*TagDeleteResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var tagID int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM tags WHERE name = ?", tagName).Scan(&tagID); err != nil {
		return nil, fmt.Errorf("tag %q: %w", tagName, model.ErrNotFound)
	}
	rows, err := tx.QueryContext(ctx, "SELECT id, entity_type FROM taggings WHERE tag_id = ? AND deleted_at IS NULL ORDER BY id", tagID)
	if err != nil {
		return nil, fmt.Errorf("list taggings: %w", err)
	}
	res := &TagDeleteResult{Tag: tagName, Removed: map[string]int{}}
	for rows.Next() {
		var id int64
		var typ string
		if err := rows.Scan(&id, &typ); err != nil {
			rows.Close()
			return nil, err
		}
		res.TaggingIDs = append(res.TaggingIDs, id)
		res.Removed[typ]++
	}
	rows.Close()
	if len(res.TaggingIDs) == 0 {
		return nil, fmt.Errorf("tag %q: %w", tagName, model.ErrNotFound)
	}
	if err := touchTagged(ctx, tx, tagID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE taggings SET deleted_at = datetime('now'), updated_at = datetime('now')
		WHERE tag_id = ? AND deleted_at IS NULL`, tagID); err != nil {
		return nil, fmt.Errorf("untag: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return res, nil
}

// UndoDelete puts the taggings of a deleted tag back (same rows, same uuids). A tagging that was
// made again since stays as it is.
func (r *TagRepo) UndoDelete(ctx context.Context, res *TagDeleteResult) error {
	return r.setTaggingsDeleted(ctx, res.TaggingIDs, false)
}

// RedoDelete untags again what UndoDelete put back.
func (r *TagRepo) RedoDelete(ctx context.Context, res *TagDeleteResult) error {
	return r.setTaggingsDeleted(ctx, res.TaggingIDs, true)
}

func (r *TagRepo) setTaggingsDeleted(ctx context.Context, ids []int64, deleted bool) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, id := range ids {
		var typ string
		var eid, tagID int64
		var isDead bool
		if err := tx.QueryRowContext(ctx, "SELECT entity_type, entity_id, tag_id, deleted_at IS NOT NULL FROM taggings WHERE id = ?", id).Scan(&typ, &eid, &tagID, &isDead); err != nil {
			continue
		}
		if isDead == deleted {
			continue
		}
		if !deleted {
			var live int
			if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM taggings WHERE tag_id = ? AND entity_type = ? AND entity_id = ? AND deleted_at IS NULL", tagID, typ, eid).Scan(&live); err != nil {
				return err
			}
			if live > 0 {
				continue
			}
		}
		val := "NULL"
		if deleted {
			val = "datetime('now')"
		}
		if _, err := tx.ExecContext(ctx, "UPDATE taggings SET deleted_at = "+val+", updated_at = datetime('now') WHERE id = ?", id); err != nil {
			return fmt.Errorf("update tagging: %w", err)
		}
		if table, ok := entityTables[typ]; ok {
			if _, err := tx.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET updated_at = datetime('now') WHERE id = ?", table), eid); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
