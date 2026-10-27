package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// DeviceID returns this database's device id from the meta table.
func DeviceID(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) (string, error) {
	var id sql.NullString
	err := q.QueryRowContext(ctx, "SELECT value FROM meta WHERE key = 'device_id'").Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read device id: %w", err)
	}
	return id.String, nil
}

// Restore brings an archived person back. It fails with ErrNotFound if the person
// does not exist or is not archived.
func (r *PersonRepo) Restore(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx,
		"UPDATE people SET archived = 0, updated_at = datetime('now') WHERE id = ? AND archived = 1", id)
	if err != nil {
		return fmt.Errorf("restore person %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("archived person %d: %w", id, model.ErrNotFound)
	}
	return nil
}

// Purge permanently deletes an already archived person and their dependent rows in
// one transaction. A tombstone is recorded for every deleted synced row.
//
// Dependents: contact methods, memberships, person links, task and interaction links, taggings and
// custom fields are deleted. Interactions and tasks linked only to this person are
// deleted too. Deals and tasks shared with others keep existing and lose the person link.
func (r *PersonRepo) Purge(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var personUUID string
	var archived int
	err = tx.QueryRowContext(ctx, "SELECT uuid, archived FROM people WHERE id = ?", id).Scan(&personUUID, &archived)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("person %d: %w", id, model.ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("find person %d: %w", id, err)
	}
	if archived == 0 {
		return fmt.Errorf("person %d is not archived, archive it first: %w", id, model.ErrConflict)
	}

	device, err := DeviceID(ctx, tx)
	if err != nil {
		return err
	}

	tombstone := func(table, where string, args ...any) error {
		_, err := tx.ExecContext(ctx,
			"INSERT OR REPLACE INTO tombstones (table_name, uuid, device_id) SELECT '"+table+"', uuid, ? FROM "+table+" WHERE "+where,
			append([]any{device}, args...)...)
		if err != nil {
			return fmt.Errorf("tombstone %s: %w", table, err)
		}
		return nil
	}
	exec := func(query string, args ...any) error {
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("purge person %d: %w", id, err)
		}
		return nil
	}

	// Interactions linked only to this person.
	soleInteractions := `id IN (SELECT source_id FROM activity_target WHERE source_kind = 'interaction' AND deleted_at IS NULL
		GROUP BY source_id HAVING COUNT(*) = 1 AND MIN(target_kind) = 'person' AND MAX(target_id) = ?)`
	if err := tombstone("interactions", soleInteractions, id); err != nil {
		return err
	}
	if err := exec("DELETE FROM external_ids WHERE entity = 'interaction' AND local_id IN (SELECT id FROM interactions WHERE "+soleInteractions+")", id); err != nil {
		return err
	}
	if err := exec("DELETE FROM interactions WHERE "+soleInteractions, id); err != nil {
		return err
	}

	// Tasks owned only by this person.
	soleTasks := `id IN (SELECT source_id FROM activity_target WHERE source_kind = 'task' AND target_kind = 'person' AND target_id = ?)
		AND NOT EXISTS (SELECT 1 FROM activity_target a WHERE a.source_kind = 'task' AND a.source_id = tasks.id
		                AND NOT (a.target_kind = 'person' AND a.target_id = ?))`
	if err := tombstone("tasks", soleTasks, id, id); err != nil {
		return err
	}
	if err := exec("DELETE FROM tasks WHERE "+soleTasks, id, id); err != nil {
		return err
	}
	// Links of the person are deleted with it (foreign keys and triggers). Record them first.
	if err := tombstone("membership", "person_id = ?", id); err != nil {
		return err
	}
	if err := tombstone("person_relation", "a_id = ? OR b_id = ?", id, id); err != nil {
		return err
	}
	if err := tombstone("deal_person", "person_id = ?", id); err != nil {
		return err
	}
	if err := tombstone("activity_target", "target_kind = 'person' AND target_id = ?", id); err != nil {
		return err
	}

	if err := tombstone("taggings", "entity_type = 'person' AND entity_id = ?", id); err != nil {
		return err
	}
	if err := exec("DELETE FROM taggings WHERE entity_type = 'person' AND entity_id = ?", id); err != nil {
		return err
	}
	if err := exec("DELETE FROM custom_fields WHERE entity_type = 'person' AND entity_id = ?", id); err != nil {
		return err
	}
	if err := exec("DELETE FROM external_ids WHERE entity = 'person' AND local_id = ?", id); err != nil {
		return err
	}
	if err := exec("DELETE FROM field_versions WHERE entity = 'task' AND local_id NOT IN (SELECT id FROM tasks)"); err != nil {
		return err
	}
	if err := exec("DELETE FROM field_versions WHERE entity = 'person' AND local_id = ?", id); err != nil {
		return err
	}
	// Cascades: person_emails, person_phones, person_handles, membership, person_relation,
	// deal_person; a trigger removes the activity_target rows and merge aliases.
	if err := tombstone("people", "id = ?", id); err != nil {
		return err
	}
	if err := exec("DELETE FROM people WHERE id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}
