package replicasql

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/replica"
)

// Device is a row of replica_devices: this database, a peer it connects to, or a device enrolled
// at it. Token is never printed.
type Device struct {
	Device     string                `json:"device"`
	Name       string                `json:"name"`
	Self       bool                  `json:"self"`
	URL        string                `json:"url,omitempty"`
	Token      string                `json:"-"`
	Role       string                `json:"role"`
	EnrolledAt string                `json:"enrolled_at"`
	RetiredAt  string                `json:"retired_at,omitempty"`
	Vector     replica.VersionVector `json:"cursor,omitempty"`
	Head       int64                 `json:"head"`
	LastSeenAt string                `json:"last_seen_at,omitempty"`
	LastPullAt string                `json:"last_pull_at,omitempty"`
	LastPushAt string                `json:"last_push_at,omitempty"`
	LastError  string                `json:"last_error,omitempty"`
}

const deviceCols = `device, name, self, COALESCE(url, ''), COALESCE(token, ''), role, enrolled_at, COALESCE(retired_at, ''), vector, head,
	COALESCE(last_seen_at, ''), COALESCE(last_pull_at, ''), COALESCE(last_push_at, ''), COALESCE(last_error, '')`

func scanDevices(rows *sql.Rows) ([]Device, error) {
	defer rows.Close()
	var out []Device
	for rows.Next() {
		var d Device
		var self int
		var vec string
		if err := rows.Scan(&d.Device, &d.Name, &self, &d.URL, &d.Token, &d.Role, &d.EnrolledAt, &d.RetiredAt, &vec, &d.Head,
			&d.LastSeenAt, &d.LastPullAt, &d.LastPushAt, &d.LastError); err != nil {
			return nil, err
		}
		d.Self = self == 1
		d.Vector = replica.VersionVector{}
		_ = json.Unmarshal([]byte(vec), &d.Vector)
		out = append(out, d)
	}
	return out, rows.Err()
}

// Devices lists the known devices, this one first.
func (a *Adapter) Devices(ctx context.Context) ([]Device, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT `+deviceCols+` FROM replica_devices ORDER BY self DESC, name, device`)
	if err != nil {
		return nil, err
	}
	return scanDevices(rows)
}

// FindDevice finds a device by name or by id (or a unique id prefix).
func (a *Adapter) FindDevice(ctx context.Context, nameOrID string) (Device, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT `+deviceCols+` FROM replica_devices WHERE name = ? OR device = ? OR device LIKE ? || '%'`, nameOrID, nameOrID, nameOrID)
	if err != nil {
		return Device{}, err
	}
	list, err := scanDevices(rows)
	if err != nil {
		return Device{}, err
	}
	for _, d := range list {
		if d.Name == nameOrID || d.Device == nameOrID {
			return d, nil
		}
	}
	switch len(list) {
	case 0:
		return Device{}, fmt.Errorf("no device named %q", nameOrID)
	case 1:
		return list[0], nil
	}
	return Device{}, fmt.Errorf("%q matches %d devices, use the full id", nameOrID, len(list))
}

// Peers lists the devices this database connects to (those with a URL), not retired.
func (a *Adapter) Peers(ctx context.Context) ([]Device, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT `+deviceCols+` FROM replica_devices WHERE url IS NOT NULL AND url <> '' AND retired_at IS NULL ORDER BY name`)
	if err != nil {
		return nil, err
	}
	return scanDevices(rows)
}

// AddPeer stores a relay or peer this database connects to. The name must be new.
func (a *Adapter) AddPeer(ctx context.Context, name, url, token, device string) error {
	if name == "" || url == "" || device == "" {
		return errors.New("a peer needs a name, a url and a device id")
	}
	if device == a.device {
		return errors.New("that is this device: the peer reported the same device id (was the database copied after sync was turned on?)")
	}
	var n int
	if err := a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM replica_devices WHERE name = ? AND device <> ?`, name, device).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("a device named %q already exists", name)
	}
	_, err := a.db.ExecContext(ctx, `INSERT INTO replica_devices (device, name, url, token, role) VALUES (?, ?, ?, ?, 'relay')
		ON CONFLICT(device) DO UPDATE SET name = excluded.name, url = excluded.url, token = excluded.token, retired_at = NULL`,
		device, name, url, token)
	return err
}

// RemovePeer forgets a connection (url and token). The device stays known if it was enrolled here.
func (a *Adapter) RemovePeer(ctx context.Context, nameOrID string) error {
	d, err := a.FindDevice(ctx, nameOrID)
	if err != nil {
		return err
	}
	if d.Self {
		return errors.New("cannot remove this device")
	}
	if d.URL == "" {
		return fmt.Errorf("%s is not a peer this device connects to, use `crm peers retire` for enrolled devices", d.Name)
	}
	res, err := a.db.ExecContext(ctx, `UPDATE replica_devices SET url = NULL, token = NULL WHERE device = ? AND EXISTS
		(SELECT 1 FROM replica_tokens WHERE device = ?)`, d.Device, d.Device)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		_, err = a.db.ExecContext(ctx, `DELETE FROM replica_devices WHERE device = ?`, d.Device)
	}
	return err
}

// Retire releases the tombstone hold of a device (for example a lost phone) and revokes its
// tokens. It is not undone by the device coming back: enroll it again with a new token.
func (a *Adapter) Retire(ctx context.Context, nameOrID string) (Device, error) {
	d, err := a.FindDevice(ctx, nameOrID)
	if err != nil {
		return d, err
	}
	if d.Self {
		return d, errors.New("cannot retire this device")
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return d, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE replica_devices SET retired_at = datetime('now'), url = NULL, token = NULL WHERE device = ?`, d.Device); err != nil {
		return d, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM replica_tokens WHERE device = ?`, d.Device); err != nil {
		return d, err
	}
	return d, tx.Commit()
}

// NoteDevice records what a device reported: its cursor and the highest sequence it has written.
// err is saved as the last error, an empty string clears it. pulled and pushed set the timestamps.
func (a *Adapter) NoteDevice(ctx context.Context, device string, vec replica.VersionVector, head int64, pulled, pushed bool, errText string) error {
	raw, _ := json.Marshal(vec)
	_, err := a.db.ExecContext(ctx, `UPDATE replica_devices SET
		vector = CASE WHEN ? <> '' AND ? <> 'null' THEN ? ELSE vector END,
		head = MAX(head, ?),
		last_seen_at = datetime('now'),
		last_pull_at = CASE WHEN ? THEN datetime('now') ELSE last_pull_at END,
		last_push_at = CASE WHEN ? THEN datetime('now') ELSE last_push_at END,
		last_error = NULLIF(?, '')
		WHERE device = ?`, string(raw), string(raw), string(raw), head, pulled, pushed, errText, device)
	return err
}

// --- tokens ---

func hashToken(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(h[:])
}

func newToken(prefix string) (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b), nil
}

// CreateEnrollToken makes a one-time token a new device uses to enroll at this relay. Only its
// hash is stored, so this is the one time it can be read.
func (a *Adapter) CreateEnrollToken(ctx context.Context, label string, ttl time.Duration) (string, error) {
	tok, err := newToken("crmenr_")
	if err != nil {
		return "", err
	}
	var exp any
	if ttl > 0 {
		exp = time.Now().UTC().Add(ttl).Format("2006-01-02 15:04:05")
	}
	_, err = a.db.ExecContext(ctx, `INSERT INTO replica_tokens (hash, kind, label, expires_at) VALUES (?, 'enroll', ?, ?)`, hashToken(tok), label, exp)
	return tok, err
}

// ErrBadToken is returned for a token that is unknown, used, expired or revoked.
var ErrBadToken = errors.New("token is not valid")

// Enroll turns a one-time enroll token into a device: it records the device and returns the
// long-lived token that device uses from now on. A device id that is already enrolled and not
// retired is refused, which catches a database that was copied after sync was turned on.
func (a *Adapter) Enroll(ctx context.Context, enrollToken, deviceID, name string) (string, error) {
	deviceID, name = strings.TrimSpace(deviceID), strings.TrimSpace(name)
	if deviceID == "" || name == "" {
		return "", errors.New("enroll needs a device id and a name")
	}
	if deviceID == a.device {
		return "", errors.New("that device id is the relay's own: a copy of the relay's database cannot enroll")
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	var kind string
	var exp, used sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT kind, expires_at, used_at FROM replica_tokens WHERE hash = ?`, hashToken(enrollToken)).Scan(&kind, &exp, &used)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (kind != "enroll" || used.Valid)) {
		return "", ErrBadToken
	}
	if err != nil {
		return "", err
	}
	if exp.Valid && exp.String < time.Now().UTC().Format("2006-01-02 15:04:05") {
		return "", fmt.Errorf("%w: expired", ErrBadToken)
	}
	var existing, retired sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT device, retired_at FROM replica_devices WHERE device = ?`, deviceID).Scan(&existing, &retired)
	if err == nil && !retired.Valid && existing.Valid {
		var has int
		_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM replica_tokens WHERE device = ?`, deviceID).Scan(&has)
		if has > 0 {
			return "", fmt.Errorf("device %s is already enrolled: two databases share one device id (run `crm peers retire` on the relay to enroll it again)", deviceID)
		}
	}
	var clash int
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM replica_devices WHERE name = ? AND device <> ?`, name, deviceID).Scan(&clash)
	if clash > 0 {
		return "", fmt.Errorf("a device named %q already exists, pick another name", name)
	}
	tok, err := newToken("crmdev_")
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE replica_tokens SET used_at = datetime('now'), device = ? WHERE hash = ?`, deviceID, hashToken(enrollToken)); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO replica_devices (device, name, role) VALUES (?, ?, 'peer')
		ON CONFLICT(device) DO UPDATE SET name = excluded.name, retired_at = NULL`, deviceID, name); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO replica_tokens (hash, kind, label, device) VALUES (?, 'device', ?, ?)`, hashToken(tok), name, deviceID); err != nil {
		return "", err
	}
	return tok, tx.Commit()
}

// Authenticate maps a device token to the device it belongs to.
func (a *Adapter) Authenticate(ctx context.Context, token string) (string, error) {
	var dev sql.NullString
	err := a.db.QueryRowContext(ctx, `SELECT t.device FROM replica_tokens t JOIN replica_devices d ON d.device = t.device
		WHERE t.hash = ? AND t.kind = 'device' AND d.retired_at IS NULL`, hashToken(token)).Scan(&dev)
	if errors.Is(err, sql.ErrNoRows) || !dev.Valid {
		return "", ErrBadToken
	}
	return dev.String, err
}
