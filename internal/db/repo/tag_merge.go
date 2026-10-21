package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// Tag rename and tag merge (open discrepancy X25). A rename changes the name of one tag and refuses
// a name another tag already has, in any case, with an error that offers a merge instead. A merge
// moves every tagging of the source tag to the target, drops the ones the target already has, and
// removes the source tag with a tombstone (tags have no deleted_at column, the same as tag delete).
// The result keeps exactly what moved, so one undo step puts the source tag and its taggings back.

// TagExistsError says a tag with that name exists. It is a conflict (exit code 4).
type TagExistsError struct {
	Name     string // the name asked for
	Existing string // the tag that already has it
	From     string // the tag being renamed
}

func (e *TagExistsError) Error() string {
	return fmt.Sprintf("a tag named %q already exists: merge #%s into #%s instead (crm tag merge %q %q)", e.Existing, e.From, e.Existing, e.From, e.Existing)
}

// Unwrap makes the error a model.ErrConflict.
func (e *TagExistsError) Unwrap() error { return model.ErrConflict }

// NormalizeTagName trims spaces and a leading #.
func NormalizeTagName(s string) string { return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "#")) }

type taggedRef struct {
	Type string
	ID   int64
}

// TagMergeResult is what a merge did, or with DryRun what it would do.
type TagMergeResult struct {
	Source     string         `json:"source"`
	Target     string         `json:"target"`
	DryRun     bool           `json:"dry_run"`
	Moved      map[string]int `json:"moved"`      // taggings moved to the target, by entity type
	Duplicates map[string]int `json:"duplicates"` // taggings the target already had, by entity type
	Total      int            `json:"total"`      // records that now carry the target because of this merge
	movedRefs  []taggedRef
	dupRefs    []taggedRef
	sourceID   int64
	sourceUUID string
	createdAt  string
}

// Label is a short description for messages and the undo history.
func (r TagMergeResult) Label() string { return fmt.Sprintf("merge tag #%s into #%s", r.Source, r.Target) }

func (r *TagRepo) idByName(ctx context.Context, q DBTX, name string) (int64, string, error) {
	var id int64
	var real string
	err := q.QueryRowContext(ctx, "SELECT id, name FROM tags WHERE name = ?", name).Scan(&id, &real)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", fmt.Errorf("tag %q: %w", name, model.ErrNotFound)
	}
	return id, real, err
}

// Rename gives a tag a new name. A name another tag has, in any case, is refused with a *TagExistsError.
// Changing only the case of the tag's own name is allowed.
func (r *TagRepo) Rename(ctx context.Context, oldName, newName string) error {
	oldName, newName = NormalizeTagName(oldName), NormalizeTagName(newName)
	if newName == "" {
		return fmt.Errorf("a tag needs a name: %w", model.ErrValidation)
	}
	id, _, err := r.idByName(ctx, r.db, oldName)
	if err != nil {
		return err
	}
	var other string
	err = r.db.QueryRowContext(ctx, "SELECT name FROM tags WHERE lower(name) = lower(?) AND id <> ?", newName, id).Scan(&other)
	if err == nil {
		return &TagExistsError{Name: newName, Existing: other, From: oldName}
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check tag name: %w", err)
	}
	if oldName == newName {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := touchTagged(ctx, tx, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE tags SET name = ?, updated_at = datetime('now') WHERE id = ?", newName, id); err != nil {
		return fmt.Errorf("rename tag: %w", err)
	}
	return tx.Commit()
}

// touchTagged bumps updated_at of every record that carries the tag, so sync sees the change.
func touchTagged(ctx context.Context, q DBTX, tagID int64) error {
	for entityType, table := range entityTables {
		if _, err := q.ExecContext(ctx,
			fmt.Sprintf(`UPDATE %s SET updated_at = datetime('now') WHERE id IN (
				SELECT entity_id FROM taggings WHERE tag_id = ? AND entity_type = ?)`, table), tagID, entityType); err != nil {
			return fmt.Errorf("touch tagged %s: %w", entityType, err)
		}
	}
	return nil
}

// MergeDryRun reports what Merge would do and writes nothing.
func (r *TagRepo) MergeDryRun(ctx context.Context, source, target string) (*TagMergeResult, error) {
	return r.merge(ctx, source, target, true)
}

// Merge moves the taggings of source to target and removes source.
func (r *TagRepo) Merge(ctx context.Context, source, target string) (*TagMergeResult, error) {
	return r.merge(ctx, source, target, false)
}

func (r *TagRepo) merge(ctx context.Context, source, target string, dryRun bool) (*TagMergeResult, error) {
	source, target = NormalizeTagName(source), NormalizeTagName(target)
	if source == "" || target == "" {
		return nil, fmt.Errorf("merge needs a source and a target tag: %w", model.ErrValidation)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	srcID, source, err := r.idByName(ctx, tx, source)
	if err != nil {
		return nil, err
	}
	tgtID, target, err := r.idByName(ctx, tx, target)
	if err != nil {
		return nil, err
	}
	if srcID == tgtID {
		return nil, fmt.Errorf("a tag cannot be merged into itself: %w", model.ErrValidation)
	}
	res := &TagMergeResult{Source: source, Target: target, DryRun: dryRun, Moved: map[string]int{}, Duplicates: map[string]int{}, sourceID: srcID}
	if err := tx.QueryRowContext(ctx, "SELECT uuid, COALESCE(created_at, datetime('now')) FROM tags WHERE id = ?", srcID).Scan(&res.sourceUUID, &res.createdAt); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT entity_type, entity_id,
			EXISTS (SELECT 1 FROM taggings x WHERE x.tag_id = ? AND x.entity_type = taggings.entity_type AND x.entity_id = taggings.entity_id)
		FROM taggings WHERE tag_id = ? ORDER BY entity_type, entity_id`, tgtID, srcID)
	if err != nil {
		return nil, fmt.Errorf("list taggings: %w", err)
	}
	for rows.Next() {
		var ref taggedRef
		var dup bool
		if err := rows.Scan(&ref.Type, &ref.ID, &dup); err != nil {
			rows.Close()
			return nil, err
		}
		if dup {
			res.dupRefs = append(res.dupRefs, ref)
			res.Duplicates[ref.Type]++
		} else {
			res.movedRefs = append(res.movedRefs, ref)
			res.Moved[ref.Type]++
		}
	}
	rows.Close()
	res.Total = len(res.movedRefs)
	if dryRun {
		return res, nil
	}
	if err := touchTagged(ctx, tx, srcID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE taggings SET tag_id = ? WHERE tag_id = ? AND NOT EXISTS (SELECT 1 FROM taggings x WHERE x.tag_id = ? AND x.entity_type = taggings.entity_type AND x.entity_id = taggings.entity_id)", tgtID, srcID, tgtID); err != nil {
		return nil, fmt.Errorf("move taggings: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM taggings WHERE tag_id = ?", srcID); err != nil {
		return nil, fmt.Errorf("drop duplicate taggings: %w", err)
	}
	device, _ := DeviceID(ctx, tx)
	if _, err := tx.ExecContext(ctx, "INSERT OR REPLACE INTO tombstones (table_name, uuid, device_id) VALUES ('tags', ?, ?)", res.sourceUUID, device); err != nil {
		return nil, fmt.Errorf("tombstone tag: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM tags WHERE id = ?", srcID); err != nil {
		return nil, fmt.Errorf("remove source tag: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE tags SET updated_at = datetime('now') WHERE id = ?", tgtID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tag merge: %w", err)
	}
	return res, nil
}

// UndoMerge brings the source tag back with its id and uuid, puts every moved tagging back on it,
// gives the duplicates their source tagging again and clears the tombstone.
func (r *TagRepo) UndoMerge(ctx context.Context, res *TagMergeResult) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	tgtID, _, err := r.idByName(ctx, tx, res.Target)
	if err != nil {
		return err
	}
	var taken int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM tags WHERE name = ? OR id = ?", res.Source, res.sourceID).Scan(&taken); err != nil {
		return err
	}
	if taken > 0 {
		return fmt.Errorf("tag %q exists again, cannot undo the merge: %w", res.Source, model.ErrConflict)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO tags (id, name, uuid, created_at, updated_at) VALUES (?, ?, ?, ?, datetime('now'))", res.sourceID, res.Source, res.sourceUUID, res.createdAt); err != nil {
		return fmt.Errorf("restore tag: %w", err)
	}
	for _, ref := range res.movedRefs {
		if _, err := tx.ExecContext(ctx, "UPDATE taggings SET tag_id = ? WHERE tag_id = ? AND entity_type = ? AND entity_id = ?", res.sourceID, tgtID, ref.Type, ref.ID); err != nil {
			return fmt.Errorf("move tagging back: %w", err)
		}
	}
	for _, ref := range res.dupRefs {
		if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO taggings (tag_id, entity_type, entity_id) VALUES (?, ?, ?)", res.sourceID, ref.Type, ref.ID); err != nil {
			return fmt.Errorf("restore tagging: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM tombstones WHERE table_name = 'tags' AND uuid = ?", res.sourceUUID); err != nil {
		return err
	}
	if err := touchTagged(ctx, tx, res.sourceID); err != nil {
		return err
	}
	return tx.Commit()
}

// RedoMerge repeats a merge that UndoMerge reversed.
func (r *TagRepo) RedoMerge(ctx context.Context, res *TagMergeResult) error {
	_, err := r.Merge(ctx, res.Source, res.Target)
	return err
}

// SortedTypes lists the entity types in a count map in a fixed order, for messages.
func SortedTypes(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
