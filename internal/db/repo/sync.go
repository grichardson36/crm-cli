package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// SyncRepo stores sync run history, cursors and the inbox of unmatched records.
type SyncRepo struct {
	db DBTX
}

// NewSyncRepo creates a new SyncRepo.
func NewSyncRepo(db DBTX) *SyncRepo {
	return &SyncRepo{db: db}
}

const syncRunColumns = "id, uuid, source, started_at, finished_at, status, created, updated, skipped, errors, note"

func scanSyncRun(row interface{ Scan(...any) error }) (*model.SyncRun, error) {
	var r model.SyncRun
	if err := row.Scan(&r.ID, &r.UUID, &r.Source, &r.StartedAt, &r.FinishedAt, &r.Status,
		&r.Created, &r.Updated, &r.Skipped, &r.Errors, &r.Note); err != nil {
		return nil, err
	}
	return &r, nil
}

// StartRun records a run in status "running".
func (r *SyncRepo) StartRun(ctx context.Context, source string) (*model.SyncRun, error) {
	res, err := r.db.ExecContext(ctx, `INSERT INTO sync_runs (uuid, source) VALUES (?, ?)`, uuid.NewString(), source)
	if err != nil {
		return nil, fmt.Errorf("start sync run: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.FindRun(ctx, id)
}

// FinishRun closes a run with its counts.
func (r *SyncRepo) FinishRun(ctx context.Context, id int64, status string, created, updated, skipped, errs int, note string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE sync_runs SET finished_at = datetime('now'), status = ?, created = ?, updated = ?, skipped = ?, errors = ?, note = ? WHERE id = ?`,
		status, created, updated, skipped, errs, nullIfEmpty(note), id)
	if err != nil {
		return fmt.Errorf("finish sync run %d: %w", id, err)
	}
	return nil
}

// FindRun returns one run.
func (r *SyncRepo) FindRun(ctx context.Context, id int64) (*model.SyncRun, error) {
	run, err := scanSyncRun(r.db.QueryRowContext(ctx, "SELECT "+syncRunColumns+" FROM sync_runs WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("sync run %d: %w", id, model.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("find sync run %d: %w", id, err)
	}
	return run, nil
}

// ListRuns lists runs newest first, optionally for one source.
func (r *SyncRepo) ListRuns(ctx context.Context, source string, limit int) ([]*model.SyncRun, error) {
	if limit <= 0 {
		limit = 20
	}
	query := "SELECT " + syncRunColumns + " FROM sync_runs"
	var args []any
	if source != "" {
		query += " WHERE source = ?"
		args = append(args, source)
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list sync runs: %w", err)
	}
	defer rows.Close()
	var out []*model.SyncRun
	for rows.Next() {
		run, err := scanSyncRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan sync run: %w", err)
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

// LastRun returns the newest run for a source, or nil when it never ran. An empty source means any.
func (r *SyncRepo) LastRun(ctx context.Context, source string) (*model.SyncRun, error) {
	runs, err := r.ListRuns(ctx, source, 1)
	if err != nil || len(runs) == 0 {
		return nil, err
	}
	return runs[0], nil
}

// Cursor returns a source's cursor, or "" when none is stored.
func (r *SyncRepo) Cursor(ctx context.Context, source string) (string, error) {
	var c sql.NullString
	err := r.db.QueryRowContext(ctx, "SELECT cursor FROM sync_cursors WHERE source = ?", source).Scan(&c)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read sync cursor: %w", err)
	}
	return c.String, nil
}

// SetCursor stores a source's cursor.
func (r *SyncRepo) SetCursor(ctx context.Context, source, cursor string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO sync_cursors (source, cursor, updated_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(source) DO UPDATE SET cursor = excluded.cursor, updated_at = excluded.updated_at`,
		source, cursor)
	if err != nil {
		return fmt.Errorf("write sync cursor: %w", err)
	}
	return nil
}

// Cursors lists all stored cursors.
func (r *SyncRepo) Cursors(ctx context.Context) ([]*model.SyncCursor, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT source, cursor, updated_at FROM sync_cursors ORDER BY source")
	if err != nil {
		return nil, fmt.Errorf("list sync cursors: %w", err)
	}
	defer rows.Close()
	var out []*model.SyncCursor
	for rows.Next() {
		var c model.SyncCursor
		if err := rows.Scan(&c.Source, &c.Cursor, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan sync cursor: %w", err)
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

// PersonState reports whether a person is archived and, if it was merged, the survivor's id (0 if none).
// It returns ErrNotFound when the person does not exist at all.
func (r *SyncRepo) PersonState(ctx context.Context, id int64) (archived bool, mergedInto int64, err error) {
	var a int
	var m sql.NullInt64
	err = r.db.QueryRowContext(ctx, "SELECT archived, merged_into FROM people WHERE id = ?", id).Scan(&a, &m)
	if errors.Is(err, sql.ErrNoRows) {
		return false, 0, fmt.Errorf("person %d: %w", id, model.ErrNotFound)
	}
	if err != nil {
		return false, 0, fmt.Errorf("person state %d: %w", id, err)
	}
	return a == 1, m.Int64, nil
}

// Inbox ------------------------------------------------------------------------------

const inboxColumns = "id, uuid, source, external_id, payload, status, created_at, updated_at"

func scanInbox(row interface{ Scan(...any) error }) (*model.SyncInboxItem, error) {
	var i model.SyncInboxItem
	if err := row.Scan(&i.ID, &i.UUID, &i.Source, &i.ExternalID, &i.Payload, &i.Status, &i.CreatedAt, &i.UpdatedAt); err != nil {
		return nil, err
	}
	return &i, nil
}

// InboxUpsert stores an unmatched record. The same source, external id and kind never makes
// two rows. A pending row gets the fresh payload. Rows already decided are left alone.
// created is true when a new row was added.
func (r *SyncRepo) InboxUpsert(ctx context.Context, source, externalID, kind, payload string) (item *model.SyncInboxItem, created bool, err error) {
	existing, err := scanInbox(r.db.QueryRowContext(ctx,
		"SELECT "+inboxColumns+" FROM sync_inbox WHERE source = ? AND external_id = ? AND json_extract(payload, '$.kind') = ?",
		source, externalID, kind))
	switch {
	case err == nil:
		if existing.Status == model.InboxPending && existing.Payload != payload {
			if _, err := r.db.ExecContext(ctx,
				"UPDATE sync_inbox SET payload = ?, updated_at = datetime('now') WHERE id = ?", payload, existing.ID); err != nil {
				return nil, false, fmt.Errorf("update inbox item: %w", err)
			}
			existing, err = r.InboxGet(ctx, existing.ID)
			return existing, false, err
		}
		return existing, false, nil
	case !errors.Is(err, sql.ErrNoRows):
		return nil, false, fmt.Errorf("find inbox item: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO sync_inbox (uuid, source, external_id, payload) VALUES (?, ?, ?, ?)",
		uuid.NewString(), source, externalID, payload)
	if err != nil {
		return nil, false, fmt.Errorf("insert inbox item: %w", err)
	}
	id, _ := res.LastInsertId()
	item, err = r.InboxGet(ctx, id)
	return item, true, err
}

// InboxGet returns one inbox item.
func (r *SyncRepo) InboxGet(ctx context.Context, id int64) (*model.SyncInboxItem, error) {
	i, err := scanInbox(r.db.QueryRowContext(ctx, "SELECT "+inboxColumns+" FROM sync_inbox WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("inbox item %d: %w", id, model.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("find inbox item %d: %w", id, err)
	}
	return i, nil
}

// InboxList lists inbox items oldest first. An empty status means every status.
func (r *SyncRepo) InboxList(ctx context.Context, status string, limit int) ([]*model.SyncInboxItem, error) {
	query := "SELECT " + inboxColumns + " FROM sync_inbox"
	var args []any
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY id"
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list inbox: %w", err)
	}
	defer rows.Close()
	var out []*model.SyncInboxItem
	for rows.Next() {
		i, err := scanInbox(rows)
		if err != nil {
			return nil, fmt.Errorf("scan inbox item: %w", err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// InboxCount counts items with a status.
func (r *SyncRepo) InboxCount(ctx context.Context, status string) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sync_inbox WHERE status = ?", status).Scan(&n); err != nil {
		return 0, fmt.Errorf("count inbox: %w", err)
	}
	return n, nil
}

// InboxSetStatus moves a pending item to a new status. It returns ErrConflict when the
// item was already decided.
func (r *SyncRepo) InboxSetStatus(ctx context.Context, id int64, status string) error {
	res, err := r.db.ExecContext(ctx,
		"UPDATE sync_inbox SET status = ?, updated_at = datetime('now') WHERE id = ? AND status = 'pending'", status, id)
	if err != nil {
		return fmt.Errorf("update inbox item %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		item, err := r.InboxGet(ctx, id)
		if err != nil {
			return err
		}
		return fmt.Errorf("inbox item %d is already %s: %w", id, item.Status, model.ErrConflict)
	}
	return nil
}
