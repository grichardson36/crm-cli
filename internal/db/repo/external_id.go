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

// DBTX is satisfied by *sql.DB and *sql.Tx so repos can join a caller's transaction.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ExternalIDRepo maps local rows to records in external systems.
type ExternalIDRepo struct {
	db DBTX
}

// NewExternalIDRepo creates a new ExternalIDRepo on a connection or a transaction.
func NewExternalIDRepo(db DBTX) *ExternalIDRepo {
	return &ExternalIDRepo{db: db}
}

// externalEntityTables lists the entities that can carry external ids.
var externalEntityTables = map[string]string{
	"person":       "people",
	"organization": "organizations",
	"deal":         "deals",
	"interaction":  "interactions",
	"task":         "tasks",
}

const externalIDColumns = "id, uuid, entity, local_id, local_uuid, source, external_id, etag, last_seen_at, created_at, updated_at"

func scanExternalID(row interface{ Scan(...any) error }) (*model.ExternalID, error) {
	var e model.ExternalID
	if err := row.Scan(&e.ID, &e.UUID, &e.Entity, &e.LocalID, &e.LocalUUID, &e.Source, &e.ExternalID,
		&e.ETag, &e.LastSeenAt, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, err
	}
	return &e, nil
}

// ExternalIDInput describes one mapping to store.
type ExternalIDInput struct {
	Entity     string
	LocalID    int64
	LocalUUID  string // looked up from the entity table when empty
	Source     string
	ExternalID string
	ETag       *string
}

// Lookup finds the mapping for an external record. It returns ErrNotFound when none exists.
func (r *ExternalIDRepo) Lookup(ctx context.Context, source, entity, externalID string) (*model.ExternalID, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT "+externalIDColumns+" FROM external_ids WHERE source = ? AND entity = ? AND external_id = ?",
		source, entity, externalID)
	e, err := scanExternalID(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("external id %s/%s/%s: %w", source, entity, externalID, model.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup external id: %w", err)
	}
	return e, nil
}

// LookupAny finds the mapping of an external id in any source, oldest first. It is how a
// calendar UID is recognised no matter which feed or file brought it in.
func (r *ExternalIDRepo) LookupAny(ctx context.Context, entity, externalID string) (*model.ExternalID, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT "+externalIDColumns+" FROM external_ids WHERE entity = ? AND external_id = ? ORDER BY id LIMIT 1", entity, externalID)
	e, err := scanExternalID(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("external id %s/%s: %w", entity, externalID, model.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup external id: %w", err)
	}
	return e, nil
}

// LookupByLocal finds the mapping for a local row in one source. It returns ErrNotFound when none exists.
func (r *ExternalIDRepo) LookupByLocal(ctx context.Context, source, entity string, localID int64) (*model.ExternalID, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT "+externalIDColumns+" FROM external_ids WHERE source = ? AND entity = ? AND local_id = ?",
		source, entity, localID)
	e, err := scanExternalID(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("external id for %s #%d in %s: %w", entity, localID, source, model.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup external id by local: %w", err)
	}
	return e, nil
}

// Upsert stores a mapping. An existing mapping for the same external record is repointed and
// its last_seen_at refreshed. It returns ErrConflict when the local row is already linked to a
// different external record in the same source.
func (r *ExternalIDRepo) Upsert(ctx context.Context, in ExternalIDInput) (*model.ExternalID, error) {
	table, ok := externalEntityTables[in.Entity]
	if !ok {
		return nil, fmt.Errorf("unknown entity %q: %w", in.Entity, model.ErrValidation)
	}
	if strings.TrimSpace(in.Source) == "" || strings.TrimSpace(in.ExternalID) == "" || in.LocalID <= 0 {
		return nil, fmt.Errorf("source, external id and local id are required: %w", model.ErrValidation)
	}
	localUUID := in.LocalUUID
	if localUUID == "" {
		// table comes from the fixed map above, never from user input.
		err := r.db.QueryRowContext(ctx, "SELECT uuid FROM "+table+" WHERE id = ?", in.LocalID).Scan(&localUUID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%s %d: %w", in.Entity, in.LocalID, model.ErrNotFound)
		}
		if err != nil {
			return nil, fmt.Errorf("find %s %d: %w", in.Entity, in.LocalID, err)
		}
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO external_ids (uuid, entity, local_id, local_uuid, source, external_id, etag, last_seen_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))
		 ON CONFLICT(source, entity, external_id) DO UPDATE SET
		   local_id = excluded.local_id, local_uuid = excluded.local_uuid, etag = excluded.etag,
		   last_seen_at = excluded.last_seen_at, updated_at = datetime('now')`,
		uuid.NewString(), in.Entity, in.LocalID, localUUID, in.Source, in.ExternalID, in.ETag)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, fmt.Errorf("%s #%d is already linked to another %s record: %w", in.Entity, in.LocalID, in.Source, model.ErrConflict)
		}
		return nil, fmt.Errorf("upsert external id: %w", err)
	}
	return r.Lookup(ctx, in.Source, in.Entity, in.ExternalID)
}

// Delete removes a mapping. The local row is untouched. It returns ErrNotFound when none exists.
func (r *ExternalIDRepo) Delete(ctx context.Context, source, entity, externalID string) error {
	res, err := r.db.ExecContext(ctx,
		"DELETE FROM external_ids WHERE source = ? AND entity = ? AND external_id = ?", source, entity, externalID)
	if err != nil {
		return fmt.Errorf("delete external id: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("external id %s/%s/%s: %w", source, entity, externalID, model.ErrNotFound)
	}
	return nil
}

// ListBySource lists a source's mappings, optionally limited to one entity.
func (r *ExternalIDRepo) ListBySource(ctx context.Context, source, entity string) ([]*model.ExternalID, error) {
	query := "SELECT " + externalIDColumns + " FROM external_ids WHERE source = ?"
	args := []any{source}
	if entity != "" {
		query += " AND entity = ?"
		args = append(args, entity)
	}
	query += " ORDER BY id"
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list external ids: %w", err)
	}
	defer rows.Close()
	var out []*model.ExternalID
	for rows.Next() {
		e, err := scanExternalID(rows)
		if err != nil {
			return nil, fmt.Errorf("scan external id: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
