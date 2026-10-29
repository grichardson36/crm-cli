package replicasql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/replica"
)

// Adapter is the SQLite implementation of replica.Adapter.
type Adapter struct {
	db     *sql.DB
	reg    *Registry
	engine *replica.Engine
	device string
	now    func() time.Time
}

var _ replica.Adapter = (*Adapter)(nil)

// Open returns the adapter of an enabled database. tables is the registry, normally
// DefaultRegistry().
func Open(ctx context.Context, db *sql.DB, tables []Table) (*Adapter, error) {
	if !IsEnabled(db) {
		return nil, ErrNotEnabled
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	reg, err := Describe(tx, tables)
	if err != nil {
		return nil, err
	}
	var dev string
	if err := tx.QueryRowContext(ctx, `SELECT device FROM replica_clock WHERE id = 1`).Scan(&dev); err != nil {
		return nil, err
	}
	if dev == "" {
		return nil, ErrNotEnabled
	}
	_ = tx.Rollback()
	a := &Adapter{db: db, reg: reg, device: dev, now: time.Now}
	a.engine = &replica.Engine{Schema: reg.Schema(), Now: func() time.Time { return a.now() }}
	return a, nil
}

// SetNow replaces the wall clock used for drift checks, for tests.
func (a *Adapter) SetNow(now func() time.Time) { a.now = now }

// Registry returns the described tables.
func (a *Adapter) Registry() *Registry { return a.reg }

// Device implements replica.Adapter.
func (a *Adapter) Device() string { return a.device }

// DB returns the database handle.
func (a *Adapter) DB() *sql.DB { return a.db }

// withTx runs fn in a transaction. With remote set the capture triggers are off for its duration.
func (a *Adapter) withTx(ctx context.Context, remote bool, fn func(tx *sql.Tx, st *store) error) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if remote {
		if _, err := tx.ExecContext(ctx, `UPDATE replica_clock SET applying = 1 WHERE id = 1`); err != nil {
			return err
		}
	}
	if err := fn(tx, &store{q: tx, reg: a.reg}); err != nil {
		return err
	}
	if remote {
		if _, err := tx.ExecContext(ctx, `UPDATE replica_clock SET applying = 0 WHERE id = 1`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const physLayout = "2006-01-02T15:04:05.000Z"

// loadClock reads the stored clock into a replica.Clock.
func (a *Adapter) loadClock(ctx context.Context, q execer) (*replica.Clock, error) {
	var phys string
	var cnt int
	if err := q.QueryRowContext(ctx, `SELECT phys, cnt FROM replica_clock WHERE id = 1`).Scan(&phys, &cnt); err != nil {
		return nil, err
	}
	c := replica.NewClock(a.device, a.now)
	if t, err := time.Parse(physLayout, phys); err == nil {
		c.Restore(t, cnt)
	}
	return c, nil
}

func (a *Adapter) saveClock(ctx context.Context, q execer, c *replica.Clock) error {
	t, cnt := c.Last()
	_, err := q.ExecContext(ctx, `UPDATE replica_clock SET phys = ?, cnt = ? WHERE id = 1`, t.UTC().Format(physLayout), cnt)
	return err
}

// ApplyBatch implements replica.Adapter.
func (a *Adapter) ApplyBatch(ctx context.Context, cs []replica.Change) (replica.BatchResult, error) {
	var res replica.BatchResult
	err := a.withTx(ctx, true, func(tx *sql.Tx, st *store) error {
		r, err := a.engine.ApplyBatch(ctx, st, st, cs)
		res = r
		if err != nil {
			return err
		}
		if r.MaxHLC != "" {
			clock, err := a.loadClock(ctx, tx)
			if err != nil {
				return err
			}
			if _, err := clock.Recv(r.MaxHLC); err != nil && !errors.Is(err, replica.ErrDrift) {
				return err
			}
			return a.saveClock(ctx, tx, clock)
		}
		return nil
	})
	return res, err
}

// Apply implements replica.Adapter.
func (a *Adapter) Apply(ctx context.Context, c replica.Change) (replica.Outcome, error) {
	res, err := a.ApplyBatch(ctx, []replica.Change{c})
	if err != nil {
		return "", err
	}
	switch {
	case len(res.Rejected) > 0:
		return "", errors.New(res.Rejected[0].Reason)
	case res.Duplicates > 0:
		return replica.Duplicate, nil
	case res.Unknown > 0:
		return replica.Unknown, nil
	}
	return replica.Applied, nil
}

// RetryUnknown applies stored changes whose collection and field the registry knows now.
func (a *Adapter) RetryUnknown(ctx context.Context) (int, error) {
	n := 0
	err := a.withTx(ctx, true, func(_ *sql.Tx, st *store) error {
		var err error
		n, err = a.engine.RetryUnknown(ctx, st, st)
		return err
	})
	return n, err
}

// Reproject rewrites the listed records, or every pending one, from their stored changes.
func (a *Adapter) Reproject(ctx context.Context, refs ...replica.RecordRef) error {
	return a.withTx(ctx, true, func(_ *sql.Tx, st *store) error {
		return a.engine.Reproject(ctx, st, st, refs...)
	})
}

// BeginRemote implements replica.Adapter. Use it around writes that must not be captured.
func (a *Adapter) BeginRemote(ctx context.Context) error {
	_, err := a.db.ExecContext(ctx, `UPDATE replica_clock SET applying = 1 WHERE id = 1`)
	return err
}

// EndRemote implements replica.Adapter.
func (a *Adapter) EndRemote(ctx context.Context) error {
	_, err := a.db.ExecContext(ctx, `UPDATE replica_clock SET applying = 0 WHERE id = 1`)
	return err
}

// EmitLocal implements replica.Adapter. Local writes to registered tables are captured by
// triggers, so this is for changes that have no row write behind them, such as a restore or a
// delete decided in the review queue. It also rewrites the record from its changes.
func (a *Adapter) EmitLocal(ctx context.Context, c replica.Change) (replica.Change, error) {
	var out replica.Change
	err := a.withTx(ctx, true, func(tx *sql.Tx, st *store) error {
		clock, err := a.loadClock(ctx, tx)
		if err != nil {
			return err
		}
		hlc, err := clock.Now()
		if err != nil {
			return err
		}
		vec, err := a.vectorOf(ctx, tx)
		if err != nil {
			return err
		}
		c.ID = replica.ChangeID{Device: a.device, Seq: vec[a.device] + 1}
		c.HLC, c.SchemaVersion, c.Status = hlc, SchemaVersion, replica.StatusApplied
		if c.Op == replica.OpDelete || a.keepBoth(c) {
			c.Seen = vec
		}
		if err := c.Validate(); err != nil {
			return err
		}
		if err := st.Put(ctx, c); err != nil {
			return err
		}
		if err := st.Bump(ctx, a.device, c.ID.Seq); err != nil {
			return err
		}
		if err := a.saveClock(ctx, tx, clock); err != nil {
			return err
		}
		out = c
		return a.engine.Reproject(ctx, st, st, replica.RecordRef{Collection: c.Collection, Key: c.Key})
	})
	return out, err
}

func (a *Adapter) keepBoth(c replica.Change) bool {
	col, ok := a.engine.Schema.Collection(c.Collection)
	if !ok {
		return false
	}
	f, ok := col.Field(c.Field)
	return ok && f.Policy == replica.KeepBoth
}

// ReadFieldClock implements replica.Adapter.
func (a *Adapter) ReadFieldClock(ctx context.Context, collection, key, field string) (replica.FieldClock, bool, error) {
	st := &store{q: a.db, reg: a.reg}
	entries, err := st.Entries(ctx, collection, key)
	if err != nil {
		return replica.FieldClock{}, false, err
	}
	r, ok := a.engine.View(collection, key, entries).Fields[field]
	if !ok {
		return replica.FieldClock{}, false, nil
	}
	return replica.FieldClock{HLC: r.HLC, Device: r.Device, Seq: r.Seq}, true, nil
}

func (a *Adapter) vectorOf(ctx context.Context, q execer) (replica.VersionVector, error) {
	rows, err := q.QueryContext(ctx, `SELECT device, seq FROM replica_vector`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	v := replica.VersionVector{}
	for rows.Next() {
		var d string
		var n int64
		if err := rows.Scan(&d, &n); err != nil {
			return nil, err
		}
		v[d] = n
	}
	return v, rows.Err()
}

// Vector implements replica.Adapter.
func (a *Adapter) Vector(ctx context.Context) (replica.VersionVector, error) {
	return a.vectorOf(ctx, a.db)
}

// Snapshot implements replica.Adapter.
func (a *Adapter) Snapshot(ctx context.Context) (replica.Snapshot, error) {
	var s replica.Snapshot
	err := a.withTx(ctx, false, func(tx *sql.Tx, _ *store) error {
		v, err := a.vectorOf(ctx, tx)
		if err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, changeSelect+` ORDER BY device, seq`)
		if err != nil {
			return err
		}
		cs, err := scanChanges(rows)
		if err != nil {
			return err
		}
		s = replica.Snapshot{Changes: cs, Vector: v}
		return nil
	})
	return s, err
}

// Pull implements replica.Adapter.
func (a *Adapter) Pull(ctx context.Context, cursor replica.VersionVector, limit int) ([]replica.Change, error) {
	if limit <= 0 {
		limit = 1000
	}
	vec, err := a.Vector(ctx)
	if err != nil {
		return nil, err
	}
	devices := make([]string, 0, len(vec))
	for d, n := range vec {
		if n > cursor[d] {
			devices = append(devices, d)
		}
	}
	sort.Strings(devices)
	var out []replica.Change
	for _, d := range devices {
		if len(out) >= limit {
			break
		}
		rows, err := a.db.QueryContext(ctx, changeSelect+` WHERE device = ? AND seq > ? ORDER BY seq LIMIT ?`, d, cursor[d], limit-len(out))
		if err != nil {
			return nil, err
		}
		cs, err := scanChanges(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cs...)
	}
	return out, nil
}

// --- review queue ---

// Review is one delete-versus-edit decision waiting for a person.
type Review struct {
	ID         int64  `json:"id"`
	Collection string `json:"collection"`
	Key        string `json:"key"`
	Label      string `json:"label"`
	Detail     string `json:"detail"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
	ResolvedAt string `json:"resolved_at,omitempty"`
}

var labelSQL = map[string]string{
	"people":        `SELECT trim(first_name || ' ' || COALESCE(last_name, '')) FROM people WHERE uuid = ?`,
	"organizations": `SELECT name FROM organizations WHERE uuid = ?`,
	"deals":         `SELECT title FROM deals WHERE uuid = ?`,
	"tasks":         `SELECT title FROM tasks WHERE uuid = ?`,
	"interactions":  `SELECT COALESCE(subject, substr(content, 1, 40), type) FROM interactions WHERE uuid = ?`,
	"tags":          `SELECT name FROM tags WHERE uuid = ?`,
}

// Reviews lists open review items, or all of them.
func (a *Adapter) Reviews(ctx context.Context, all bool) ([]Review, error) {
	query := `SELECT id, collection, key, detail, status, created_at, COALESCE(resolved_at, '') FROM replica_review`
	if !all {
		query += ` WHERE status = 'open'`
	}
	rows, err := a.db.QueryContext(ctx, query+` ORDER BY id`)
	if err != nil {
		return nil, err
	}
	var out []Review
	for rows.Next() {
		var r Review
		if err := rows.Scan(&r.ID, &r.Collection, &r.Key, &r.Detail, &r.Status, &r.CreatedAt, &r.ResolvedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	_ = rows.Close()
	for i := range out {
		out[i].Label = out[i].Key
		if qs, ok := labelSQL[out[i].Collection]; ok {
			var l sql.NullString
			if err := a.db.QueryRowContext(ctx, qs, out[i].Key).Scan(&l); err == nil && l.String != "" {
				out[i].Label = l.String
			}
		}
	}
	return out, rows.Err()
}

// Resolve settles a review item. Keeping the record emits a restore, so every device brings the
// record back with the edits. Deleting it emits a new delete that has seen the edits, so every
// device removes it for good. Either way the other devices close their own copy of the item.
func (a *Adapter) Resolve(ctx context.Context, id int64, keep bool) error {
	var r Review
	err := a.db.QueryRowContext(ctx, `SELECT id, collection, key, status FROM replica_review WHERE id = ?`, id).Scan(&r.ID, &r.Collection, &r.Key, &r.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("review item %d not found", id)
	}
	if err != nil {
		return err
	}
	if r.Status != "open" {
		return fmt.Errorf("review item %d is already %s", id, r.Status)
	}
	status, op := "deleted", replica.OpDelete
	if keep {
		status, op = "kept", replica.OpRestore
	}
	if _, err := a.db.ExecContext(ctx, `UPDATE replica_review SET status = ?, resolved_at = datetime('now') WHERE id = ?`, status, id); err != nil {
		return err
	}
	_, err = a.EmitLocal(ctx, replica.Change{Collection: r.Collection, Key: r.Key, Op: op})
	return err
}

// --- status ---

// Status is a summary for `crm peers status`.
type Status struct {
	Device         string                `json:"device"`
	SchemaVersion  int                   `json:"schema_version"`
	Vector         replica.VersionVector `json:"cursor"`
	Changes        int64                 `json:"changes"`
	Unknown        int64                 `json:"unknown_changes"`
	OpenReviews    int64                 `json:"open_reviews"`
	Pending        []PendingRecord       `json:"pending,omitempty"`
	Tables         []string              `json:"tables"`
	NotSynced      map[string]string     `json:"not_synced"`
	ExcludedFields map[string][]string   `json:"excluded_fields,omitempty"`
}

// PendingRecord is a record that could not be written yet.
type PendingRecord struct {
	Collection string `json:"collection"`
	Key        string `json:"key"`
	Reason     string `json:"reason"`
}

// Status reports the sync state of the database.
func (a *Adapter) Status(ctx context.Context) (Status, error) {
	s := Status{Device: a.device, SchemaVersion: SchemaVersion, Tables: a.reg.Order, NotSynced: NotCovered()}
	var err error
	if s.Vector, err = a.Vector(ctx); err != nil {
		return s, err
	}
	_ = a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM replica_changes`).Scan(&s.Changes)
	_ = a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM replica_changes WHERE status = 'unknown'`).Scan(&s.Unknown)
	_ = a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM replica_review WHERE status = 'open'`).Scan(&s.OpenReviews)
	rows, err := a.db.QueryContext(ctx, `SELECT collection, key, reason FROM replica_pending ORDER BY since LIMIT 50`)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var p PendingRecord
		if err := rows.Scan(&p.Collection, &p.Key, &p.Reason); err != nil {
			return s, err
		}
		s.Pending = append(s.Pending, p)
	}
	for n, d := range a.reg.Tables {
		if len(d.Excluded) > 0 {
			if s.ExcludedFields == nil {
				s.ExcludedFields = map[string][]string{}
			}
			s.ExcludedFields[n] = d.Excluded
		}
	}
	return s, rows.Err()
}

// PurgeTombstones removes delete records, with the history of the records they delete, once every
// enrolled device has passed them (contract rule 9). A device counts as passed when its reported
// cursor includes the delete and this database holds everything the device had written when it
// reported. Retired devices are ignored. With no other device enrolled nothing is purged, because
// a device enrolled later could bring the record back. Records in the review queue are kept.
func (a *Adapter) PurgeTombstones(ctx context.Context) (int, error) {
	n := 0
	err := a.withTx(ctx, false, func(tx *sql.Tx, st *store) error {
		rows, err := tx.QueryContext(ctx, `SELECT device, vector, head FROM replica_devices WHERE self = 0 AND retired_at IS NULL`)
		if err != nil {
			return err
		}
		type peer struct {
			vec  replica.VersionVector
			head int64
			dev  string
		}
		var peers []peer
		for rows.Next() {
			var p peer
			var raw string
			if err := rows.Scan(&p.dev, &raw, &p.head); err != nil {
				_ = rows.Close()
				return err
			}
			p.vec = replica.VersionVector{}
			if err := unmarshalVector(raw, &p.vec); err != nil {
				_ = rows.Close()
				return err
			}
			peers = append(peers, p)
		}
		_ = rows.Close()
		if len(peers) == 0 {
			return nil
		}
		mine, err := a.vectorOf(ctx, tx)
		if err != nil {
			return err
		}
		drows, err := tx.QueryContext(ctx, changeSelect+` WHERE field = '' AND op = 'delete'`)
		if err != nil {
			return err
		}
		dels, err := scanChanges(drows)
		if err != nil {
			return err
		}
	next:
		for _, d := range dels {
			for _, p := range peers {
				if !p.vec.Covers(d.ID.Device, d.ID.Seq) || mine[p.dev] < p.head {
					continue next
				}
			}
			entries, err := st.Entries(ctx, d.Collection, d.Key)
			if err != nil {
				return err
			}
			if v := a.engine.View(d.Collection, d.Key, entries); v.Life != replica.Deleted {
				continue
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM replica_changes WHERE collection = ? AND key = ?`, d.Collection, d.Key); err != nil {
				return err
			}
			_, _ = tx.ExecContext(ctx, `DELETE FROM replica_pending WHERE collection = ? AND key = ?`, d.Collection, d.Key)
			n++
		}
		return nil
	})
	return n, err
}

func unmarshalVector(raw string, v *replica.VersionVector) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), v)
}
