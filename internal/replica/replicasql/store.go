package replicasql

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/replica"
)

// execer is the part of *sql.DB and *sql.Tx the store uses.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// store implements replica.Store and replica.Projector on a transaction.
type store struct {
	q   execer
	reg *Registry
}

const changeSelect = `SELECT collection, key, field, device, seq, hlc, op, value, schema_version, tx, seen, status FROM replica_changes`

func scanChanges(rows *sql.Rows) ([]replica.Change, error) {
	defer rows.Close()
	var out []replica.Change
	for rows.Next() {
		var c replica.Change
		var value, tx, seen sql.NullString
		var op string
		if err := rows.Scan(&c.Collection, &c.Key, &c.Field, &c.ID.Device, &c.ID.Seq, &c.HLC, &op, &value, &c.SchemaVersion, &tx, &seen, &c.Status); err != nil {
			return nil, err
		}
		c.Op = replica.Op(op)
		if value.Valid {
			c.Value = json.RawMessage(value.String)
		}
		c.Tx = tx.String
		if seen.Valid && seen.String != "" {
			if err := json.Unmarshal([]byte(seen.String), &c.Seen); err != nil {
				return nil, fmt.Errorf("seen of %s#%d: %w", c.ID.Device, c.ID.Seq, err)
			}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *store) Get(ctx context.Context, coll, key, field, device string) (*replica.Change, error) {
	rows, err := s.q.QueryContext(ctx, changeSelect+` WHERE collection = ? AND key = ? AND field = ? AND device = ?`, coll, key, field, device)
	if err != nil {
		return nil, err
	}
	list, err := scanChanges(rows)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}

func (s *store) Entries(ctx context.Context, coll, key string) ([]replica.Change, error) {
	rows, err := s.q.QueryContext(ctx, changeSelect+` WHERE collection = ? AND key = ?`, coll, key)
	if err != nil {
		return nil, err
	}
	return scanChanges(rows)
}

func (s *store) Put(ctx context.Context, c replica.Change) error {
	var value, tx, seen any
	if len(c.Value) > 0 {
		value = string(c.Value)
	}
	if c.Tx != "" {
		tx = c.Tx
	}
	if len(c.Seen) > 0 {
		b, _ := json.Marshal(c.Seen)
		seen = string(b)
	}
	status := c.Status
	if status == "" {
		status = replica.StatusApplied
	}
	_, err := s.q.ExecContext(ctx, `INSERT OR REPLACE INTO replica_changes
		(collection, key, field, device, seq, hlc, op, value, schema_version, tx, seen, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Collection, c.Key, c.Field, c.ID.Device, c.ID.Seq, c.HLC, string(c.Op), value, c.SchemaVersion, tx, seen, status)
	return err
}

func (s *store) Bump(ctx context.Context, device string, seq int64) error {
	_, err := s.q.ExecContext(ctx, `INSERT INTO replica_vector (device, seq) VALUES (?, ?)
		ON CONFLICT(device) DO UPDATE SET seq = MAX(seq, excluded.seq)`, device, seq)
	return err
}

func (s *store) SyncReview(ctx context.Context, ref replica.RecordRef, parked bool, detail string) error {
	if parked {
		_, err := s.q.ExecContext(ctx, `INSERT INTO replica_review (collection, key, detail) VALUES (?, ?, ?)
			ON CONFLICT(collection, key) WHERE status = 'open' DO UPDATE SET detail = excluded.detail`, ref.Collection, ref.Key, detail)
		return err
	}
	_, err := s.q.ExecContext(ctx, `UPDATE replica_review SET status = 'auto', resolved_at = datetime('now')
		WHERE collection = ? AND key = ? AND status = 'open'`, ref.Collection, ref.Key)
	return err
}

func (s *store) Pending(ctx context.Context) ([]replica.RecordRef, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT collection, key FROM replica_pending`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []replica.RecordRef
	for rows.Next() {
		var r replica.RecordRef
		if err := rows.Scan(&r.Collection, &r.Key); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *store) SetPending(ctx context.Context, ref replica.RecordRef, reason string) error {
	if reason == "" {
		_, err := s.q.ExecContext(ctx, `DELETE FROM replica_pending WHERE collection = ? AND key = ?`, ref.Collection, ref.Key)
		return err
	}
	_, err := s.q.ExecContext(ctx, `INSERT INTO replica_pending (collection, key, reason) VALUES (?, ?, ?)
		ON CONFLICT(collection, key) DO UPDATE SET reason = excluded.reason`, ref.Collection, ref.Key, reason)
	return err
}

func (s *store) Unknown(ctx context.Context) ([]replica.Change, error) {
	rows, err := s.q.QueryContext(ctx, changeSelect+` WHERE status = 'unknown'`)
	if err != nil {
		return nil, err
	}
	return scanChanges(rows)
}

// --- projector ---

func notReady(format string, a ...any) error {
	return fmt.Errorf("%w: %s", replica.ErrNotReady, fmt.Sprintf(format, a...))
}

// Project writes a record into its table. A live or parked record is created or updated, a deleted
// one is removed. Anything the table refuses leaves the record pending with the reason.
func (s *store) Project(ctx context.Context, v replica.RecordView) error {
	d, ok := s.reg.Tables[v.Collection]
	if !ok {
		return nil
	}
	if v.Life == replica.Deleted {
		if _, err := s.q.ExecContext(ctx, `DELETE FROM `+q(d.Name)+` WHERE uuid = ?`, v.Key); err != nil {
			return notReady("delete %s: %v", d.Name, err)
		}
		return nil
	}
	return s.upsert(ctx, d, v)
}

func (s *store) lookupID(ctx context.Context, table, uuid string) (int64, error) {
	var id int64
	err := s.q.QueryRowContext(ctx, `SELECT id FROM `+q(table)+` WHERE uuid = ?`, uuid).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, notReady("waiting for %s %s", table, uuid)
	}
	return id, err
}

// decodeScalar turns a JSON value into a SQL parameter.
func decodeScalar(raw json.RawMessage) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	switch x := v.(type) {
	case nil:
		return nil, nil
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i, nil
		}
		f, err := x.Float64()
		return f, err
	case bool:
		if x {
			return int64(1), nil
		}
		return int64(0), nil
	case string:
		return x, nil
	}
	return string(raw), nil
}

func (s *store) fieldValue(ctx context.Context, d *Described, c Column, v replica.RecordView, r replica.Resolved) (any, error) {
	if r.Op != replica.OpSet {
		return nil, nil
	}
	switch {
	case c.Poly != nil:
		kr, ok := v.Fields[c.Poly.KindColumn]
		if !ok || kr.Op != replica.OpSet {
			return nil, notReady("%s.%s waits for %s", d.Name, c.Name, c.Poly.KindColumn)
		}
		var kind string
		if err := json.Unmarshal(kr.Value, &kind); err != nil {
			return nil, notReady("%s.%s: bad %s", d.Name, c.Name, c.Poly.KindColumn)
		}
		target, ok := c.Poly.Targets[kind]
		if !ok {
			return nil, notReady("%s.%s: unknown kind %q", d.Name, c.Name, kind)
		}
		var u string
		if err := json.Unmarshal(r.Value, &u); err != nil {
			return nil, notReady("%s.%s: bad reference", d.Name, c.Name)
		}
		return s.lookupID(ctx, target, u)
	case c.Ref != "":
		var u string
		if err := json.Unmarshal(r.Value, &u); err != nil {
			return nil, notReady("%s.%s: bad reference", d.Name, c.Name)
		}
		return s.lookupID(ctx, c.Ref, u)
	}
	val, err := decodeScalar(r.Value)
	if err != nil {
		return nil, notReady("%s.%s: %v", d.Name, c.Name, err)
	}
	return val, nil
}

func (s *store) upsert(ctx context.Context, d *Described, v replica.RecordView) error {
	vals := map[string]any{}
	for _, c := range d.Columns {
		r, ok := v.Fields[c.Name]
		if !ok {
			continue
		}
		val, err := s.fieldValue(ctx, d, c, v, r)
		if err != nil {
			return err
		}
		vals[c.Name] = val
	}
	if d.Pair[0] != "" {
		a, aok := vals[d.Pair[0]].(int64)
		b, bok := vals[d.Pair[1]].(int64)
		if aok && bok && a > b {
			vals[d.Pair[0]], vals[d.Pair[1]] = b, a
		}
	}
	stamp := ""
	if d.HasUpdate {
		if h, err := replica.ParseHLC(v.LatestHLC); err == nil {
			stamp = h.Time.Format("2006-01-02 15:04:05")
		}
	}
	var exists int64
	err := s.q.QueryRowContext(ctx, `SELECT 1 FROM `+q(d.Name)+` WHERE uuid = ?`, v.Key).Scan(&exists)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		var sets []string
		var args []any
		for _, c := range d.Columns {
			if val, ok := vals[c.Name]; ok {
				sets = append(sets, q(c.Name)+" = ?")
				args = append(args, val)
			}
		}
		if len(sets) == 0 {
			return nil
		}
		if stamp != "" {
			sets = append(sets, "updated_at = ?")
			args = append(args, stamp)
		}
		if _, err := s.q.ExecContext(ctx, `UPDATE `+q(d.Name)+` SET `+strings.Join(sets, ", ")+` WHERE uuid = ?`, append(args, v.Key)...); err != nil {
			return notReady("update %s: %v", d.Name, err)
		}
		return nil
	}
	cols := []string{"uuid"}
	marks := []string{"?"}
	args := []any{v.Key}
	for _, c := range d.Columns {
		val, ok := vals[c.Name]
		if !ok {
			if c.NotNull && !c.HasDefault {
				return notReady("%s %s is missing %s", d.Name, v.Key, c.Name)
			}
			continue
		}
		if val == nil && c.NotNull {
			return notReady("%s %s: %s cannot be empty", d.Name, v.Key, c.Name)
		}
		cols = append(cols, q(c.Name))
		marks = append(marks, "?")
		args = append(args, val)
	}
	for col, literal := range d.Local {
		cols = append(cols, q(col))
		marks = append(marks, literal)
	}
	if stamp != "" {
		cols = append(cols, "updated_at")
		marks = append(marks, "?")
		args = append(args, stamp)
	}
	if _, err := s.q.ExecContext(ctx, `INSERT INTO `+q(d.Name)+` (`+strings.Join(cols, ", ")+`) VALUES (`+strings.Join(marks, ", ")+`)`, args...); err != nil {
		return notReady("insert %s: %v", d.Name, err)
	}
	return nil
}
