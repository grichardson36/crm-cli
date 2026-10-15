package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// CaldavResource is what the CalDAV adapter remembers about one remote calendar object.
type CaldavResource struct {
	Source   string
	Href     string
	UID      string
	ETag     string
	BaseHash string
	BaseJSON string
	SyncedAt string
}

// CaldavRepo stores the adapter's per-resource state.
type CaldavRepo struct {
	db *sql.DB
}

// NewCaldavRepo creates a new CaldavRepo.
func NewCaldavRepo(db *sql.DB) *CaldavRepo { return &CaldavRepo{db: db} }

// List returns every remembered resource of a source.
func (r *CaldavRepo) List(ctx context.Context, source string) ([]CaldavResource, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT source, href, uid, coalesce(etag, ''), base_hash, base_json, synced_at FROM caldav_resources WHERE source = ? ORDER BY href`, source)
	if err != nil {
		return nil, fmt.Errorf("list caldav resources: %w", err)
	}
	defer rows.Close()
	var out []CaldavResource
	for rows.Next() {
		var c CaldavResource
		if err := rows.Scan(&c.Source, &c.Href, &c.UID, &c.ETag, &c.BaseHash, &c.BaseJSON, &c.SyncedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ByUID finds the resource of a UID, or ErrNotFound.
func (r *CaldavRepo) ByUID(ctx context.Context, source, uid string) (*CaldavResource, error) {
	var c CaldavResource
	err := r.db.QueryRowContext(ctx,
		`SELECT source, href, uid, coalesce(etag, ''), base_hash, base_json, synced_at FROM caldav_resources WHERE source = ? AND uid = ?`, source, uid).
		Scan(&c.Source, &c.Href, &c.UID, &c.ETag, &c.BaseHash, &c.BaseJSON, &c.SyncedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("caldav resource for %s: %w", uid, model.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("find caldav resource: %w", err)
	}
	return &c, nil
}

// Save stores or replaces a resource.
func (r *CaldavRepo) Save(ctx context.Context, c CaldavResource) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO caldav_resources (source, href, uid, etag, base_hash, base_json, synced_at) VALUES (?, ?, ?, ?, ?, ?, datetime('now'))
		 ON CONFLICT(source, href) DO UPDATE SET uid = excluded.uid, etag = excluded.etag, base_hash = excluded.base_hash,
		   base_json = excluded.base_json, synced_at = excluded.synced_at`,
		c.Source, c.Href, c.UID, c.ETag, c.BaseHash, c.BaseJSON)
	if err != nil {
		return fmt.Errorf("save caldav resource: %w", err)
	}
	return nil
}

// Delete forgets a resource.
func (r *CaldavRepo) Delete(ctx context.Context, source, href string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM caldav_resources WHERE source = ? AND href = ?`, source, href)
	if err != nil {
		return fmt.Errorf("delete caldav resource: %w", err)
	}
	return nil
}

// EventFieldValues flattens the fields of an event that two-way sync compares.
func EventFieldValues(e *model.Event) map[string]string { return eventFieldValues(e) }

// SeriesIDs lists the ids of live events that are not overrides.
func (r *EventRepo) SeriesIDs(ctx context.Context) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM events WHERE archived = 0 AND recurrence_parent_id IS NULL ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
