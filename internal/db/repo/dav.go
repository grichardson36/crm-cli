package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// DavSource is the external_ids source name for contacts that came from or are linked to a
// device through the CardDAV server.
const DavSource = "dav"

// Meta keys the CardDAV server writes so `crm dav status` can read them from another process.
const (
	MetaDavLastRequest = "dav.last_request_at"
	MetaDavLastPut     = "dav.last_put_at"
)

// DavRepo holds the CardDAV server's queries.
type DavRepo struct {
	db *sql.DB
}

// NewDavRepo creates a new DavRepo.
func NewDavRepo(db *sql.DB) *DavRepo {
	return &DavRepo{db: db}
}

// DavListRow is one visible contact, enough to build a collection listing without reading
// the whole person.
type DavListRow struct {
	PersonID  int64
	DavID     string
	UpdatedAt string
}

// DavTarget is the person behind a device path.
type DavTarget struct {
	PersonID   int64
	UUID       string
	DavID      string
	Archived   bool
	MergedInto int64
}

// ListVisible lists the active people the collection exposes. Archived and merged people are
// left out so the phone deletes them.
func (r *DavRepo) ListVisible(ctx context.Context) ([]DavListRow, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT p.id, COALESCE(x.external_id, p.uuid), p.updated_at
		 FROM people p
		 LEFT JOIN external_ids x ON x.source = 'dav' AND x.entity = 'person' AND x.local_id = p.id
		 WHERE p.archived = 0 AND p.merged_into IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("list dav contacts: %w", err)
	}
	defer rows.Close()
	var out []DavListRow
	for rows.Next() {
		var row DavListRow
		if err := rows.Scan(&row.PersonID, &row.DavID, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan dav contact: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// Collection returns a fingerprint of the visible set: count, sum of ids and sum of update
// times. It changes when a contact is added, edited, archived, merged away or restored.
func (r *DavRepo) Collection(ctx context.Context) (count, idSum, timeSum int64, err error) {
	err = r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(id), 0), COALESCE(SUM(CAST(strftime('%s', updated_at) AS INTEGER)), 0)
		 FROM people WHERE archived = 0 AND merged_into IS NULL`).Scan(&count, &idSum, &timeSum)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("dav collection fingerprint: %w", err)
	}
	return count, idSum, timeSum, nil
}

// Resolve finds the person behind a device path stem. The id is the external id the device
// chose, or the person's uuid for people that started in the CRM. It returns ErrNotFound when
// no person (active or not) has that id.
func (r *DavRepo) Resolve(ctx context.Context, davID string) (*DavTarget, error) {
	var t DavTarget
	var merged sql.NullInt64
	var archived int
	err := r.db.QueryRowContext(ctx,
		`SELECT p.id, p.uuid, p.archived, p.merged_into
		 FROM external_ids x JOIN people p ON p.id = x.local_id
		 WHERE x.source = 'dav' AND x.entity = 'person' AND x.external_id = ?`, davID).
		Scan(&t.PersonID, &t.UUID, &archived, &merged)
	if errors.Is(err, sql.ErrNoRows) {
		err = r.db.QueryRowContext(ctx,
			`SELECT p.id, p.uuid, p.archived, p.merged_into FROM people p
			 WHERE p.uuid = ? AND NOT EXISTS (
			   SELECT 1 FROM external_ids x WHERE x.source = 'dav' AND x.entity = 'person' AND x.local_id = p.id)`, davID).
			Scan(&t.PersonID, &t.UUID, &archived, &merged)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("dav contact %s: %w", davID, model.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("resolve dav contact: %w", err)
	}
	t.DavID = davID
	t.Archived = archived != 0
	t.MergedInto = merged.Int64
	return &t, nil
}

// DavIDFor returns the id the device uses for a person: its mapped external id, or its uuid.
func (r *DavRepo) DavIDFor(ctx context.Context, personID int64) (string, error) {
	var id string
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE((SELECT external_id FROM external_ids WHERE source = 'dav' AND entity = 'person' AND local_id = p.id), p.uuid)
		 FROM people p WHERE p.id = ?`, personID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("person %d: %w", personID, model.ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("dav id: %w", err)
	}
	return id, nil
}

// GetState returns the saved device state of a person, or nil when the phone never saw them.
func (r *DavRepo) GetState(ctx context.Context, personID int64) (*model.DavState, error) {
	var s model.DavState
	err := r.db.QueryRowContext(ctx,
		`SELECT person_id, base_json, etag, extra_vcard, synced_at FROM dav_state WHERE person_id = ?`, personID).
		Scan(&s.PersonID, &s.BaseJSON, &s.ETag, &s.ExtraVCard, &s.SyncedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get dav state: %w", err)
	}
	return &s, nil
}

// SaveState stores the device state of a person.
func (r *DavRepo) SaveState(ctx context.Context, s model.DavState) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO dav_state (person_id, base_json, etag, extra_vcard, synced_at)
		 VALUES (?, ?, ?, ?, datetime('now'))
		 ON CONFLICT(person_id) DO UPDATE SET
		   base_json = excluded.base_json, etag = excluded.etag,
		   extra_vcard = excluded.extra_vcard, synced_at = excluded.synced_at`,
		s.PersonID, s.BaseJSON, s.ETag, s.ExtraVCard)
	if err != nil {
		return fmt.Errorf("save dav state: %w", err)
	}
	return nil
}

// SetMeta stores a value in the meta table.
func (r *DavRepo) SetMeta(ctx context.Context, key, value string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("set meta %s: %w", key, err)
	}
	return nil
}

func (r *DavRepo) getMeta(ctx context.Context, key string) (*string, error) {
	var v sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !v.Valid) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get meta %s: %w", key, err)
	}
	return &v.String, nil
}

// Status summarizes the server's state.
func (r *DavRepo) Status(ctx context.Context) (*model.DavStatus, error) {
	var st model.DavStatus
	q := func(dest *int, query string) error {
		if err := r.db.QueryRowContext(ctx, query).Scan(dest); err != nil {
			return fmt.Errorf("dav status: %w", err)
		}
		return nil
	}
	if err := q(&st.ContactsExposed, `SELECT COUNT(*) FROM people WHERE archived = 0 AND merged_into IS NULL`); err != nil {
		return nil, err
	}
	if err := q(&st.DeviceCreated, `SELECT COUNT(*) FROM external_ids WHERE source = 'dav' AND entity = 'person'`); err != nil {
		return nil, err
	}
	if err := q(&st.SyncedToDevice, `SELECT COUNT(*) FROM dav_state s JOIN people p ON p.id = s.person_id WHERE p.archived = 0 AND p.merged_into IS NULL`); err != nil {
		return nil, err
	}
	if err := q(&st.Conflicts, `SELECT COUNT(*) FROM sync_conflicts`); err != nil {
		return nil, err
	}
	if err := q(&st.OpenConflicts, `SELECT COUNT(*) FROM sync_conflicts WHERE undone = 0 AND winner != 'link'`); err != nil {
		return nil, err
	}
	var last sql.NullString
	if err := r.db.QueryRowContext(ctx, `SELECT MAX(synced_at) FROM dav_state`).Scan(&last); err != nil {
		return nil, fmt.Errorf("dav status: %w", err)
	}
	if last.Valid {
		st.LastGetAt = &last.String
	}
	var err error
	if st.LastRequestAt, err = r.getMeta(ctx, MetaDavLastRequest); err != nil {
		return nil, err
	}
	if st.LastPutAt, err = r.getMeta(ctx, MetaDavLastPut); err != nil {
		return nil, err
	}
	return &st, nil
}

// --- conflicts ---------------------------------------------------------------------------

const conflictColumns = `c.id, c.uuid, c.person_id, TRIM(p.first_name || ' ' || COALESCE(p.last_name, '')), c.field,
	c.crm_value, c.device_value, c.winner, c.resolved_at, c.undone, c.created_at, c.updated_at`

func scanConflict(row interface{ Scan(...any) error }) (*model.SyncConflict, error) {
	var c model.SyncConflict
	var undone int
	if err := row.Scan(&c.ID, &c.UUID, &c.PersonID, &c.PersonName, &c.Field, &c.CRMValue, &c.DeviceValue,
		&c.Winner, &c.ResolvedAt, &undone, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	c.Undone = undone != 0
	return &c, nil
}

// AddConflict logs a conflict (or a link) and returns it.
func (r *DavRepo) AddConflict(ctx context.Context, personID int64, field string, crmValue, deviceValue *string, winner string) (*model.SyncConflict, error) {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO sync_conflicts (uuid, person_id, field, crm_value, device_value, winner, resolved_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), personID, field, crmValue, deviceValue, winner, now, now, now)
	if err != nil {
		return nil, fmt.Errorf("log sync conflict: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.Conflict(ctx, id)
}

// Conflict returns one logged conflict.
func (r *DavRepo) Conflict(ctx context.Context, id int64) (*model.SyncConflict, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+conflictColumns+` FROM sync_conflicts c JOIN people p ON p.id = c.person_id WHERE c.id = ?`, id)
	c, err := scanConflict(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("sync conflict %d: %w", id, model.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get sync conflict: %w", err)
	}
	return c, nil
}

// Conflicts lists logged conflicts, newest first. Undone ones are left out unless asked for.
func (r *DavRepo) Conflicts(ctx context.Context, includeUndone bool, limit int) ([]model.SyncConflict, error) {
	var where []string
	if !includeUndone {
		where = append(where, "c.undone = 0")
	}
	query := `SELECT ` + conflictColumns + ` FROM sync_conflicts c JOIN people p ON p.id = c.person_id`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY c.id DESC"
	var args []any
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list sync conflicts: %w", err)
	}
	defer rows.Close()
	out := []model.SyncConflict{}
	for rows.Next() {
		c, err := scanConflict(rows)
		if err != nil {
			return nil, fmt.Errorf("scan sync conflict: %w", err)
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// MarkConflictUndone flags a conflict as undone. It returns ErrConflict if it already was.
func (r *DavRepo) MarkConflictUndone(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE sync_conflicts SET undone = 1, updated_at = datetime('now') WHERE id = ? AND undone = 0`, id)
	if err != nil {
		return fmt.Errorf("mark conflict undone: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := r.Conflict(ctx, id); err != nil {
			return err
		}
		return fmt.Errorf("sync conflict %d is already undone: %w", id, model.ErrConflict)
	}
	return nil
}
