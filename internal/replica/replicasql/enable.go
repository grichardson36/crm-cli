package replicasql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/replica"
)

// Meta keys kept in the meta table.
const (
	metaEnabled   = "replica_enabled"
	metaDeviceID  = "replica_device_id"
	metaSignature = "replica_signature"
)

// ErrNotEnabled is returned when sync was not turned on for the database.
var ErrNotEnabled = errors.New("sync is not enabled for this database, run `crm peers init`")

// IsEnabled reports whether sync was turned on. A database without the sync tables is not enabled.
func IsEnabled(db interface {
	QueryRow(string, ...any) *sql.Row
}) bool {
	var v string
	err := db.QueryRow(`SELECT value FROM meta WHERE key = ?`, metaEnabled).Scan(&v)
	return err == nil && v == "1"
}

// EnableResult says what Enable did.
type EnableResult struct {
	Device   string `json:"device"`
	Already  bool   `json:"already_enabled"`
	Tables   int    `json:"tables"`
	Baseline int64  `json:"baseline_changes"`
}

func execAll(ctx context.Context, tx *sql.Tx, stmts []string) error {
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("%w in: %.200s", err, s)
		}
	}
	return nil
}

// install rebuilds the triggers of every registered table and writes the baseline of values that
// have no change yet. It runs in the caller's transaction.
func install(ctx context.Context, tx *sql.Tx, reg *Registry) error {
	rows, err := tx.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'trigger' AND name LIKE 'rpl\_%' ESCAPE '\'`)
	if err != nil {
		return err
	}
	var old []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			_ = rows.Close()
			return err
		}
		old = append(old, n)
	}
	_ = rows.Close()
	for _, n := range old {
		if _, err := tx.ExecContext(ctx, "DROP TRIGGER IF EXISTS "+q(n)); err != nil {
			return err
		}
	}
	for _, name := range reg.Order {
		d := reg.Tables[name]
		if err := normalizeUUIDs(ctx, tx, d); err != nil {
			return fmt.Errorf("natural keys %s: %w", name, err)
		}
		if err := execAll(ctx, tx, d.baselineSQL()); err != nil {
			return fmt.Errorf("baseline %s: %w", name, err)
		}
	}
	for _, name := range reg.Order {
		if err := execAll(ctx, tx, reg.Tables[name].triggerSQL()); err != nil {
			return fmt.Errorf("triggers %s: %w", name, err)
		}
	}
	return nil
}

// Enable turns sync on: it gives the database a sync identity, writes a baseline change for every
// current value (the rows themselves are not touched) and installs the capture triggers. Running it
// again only repairs the triggers and fills in missing baseline entries.
func Enable(ctx context.Context, db *sql.DB, tables []Table) (EnableResult, error) {
	var res EnableResult
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := ensureUUIDColumns(ctx, tx, tables); err != nil {
		return res, err
	}
	reg, err := Describe(tx, tables)
	if err != nil {
		return res, err
	}
	var dev sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, metaDeviceID).Scan(&dev)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return res, err
	}
	var en sql.NullString
	_ = tx.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, metaEnabled).Scan(&en)
	res.Already = en.String == "1"
	if !dev.Valid || dev.String == "" {
		dev = sql.NullString{String: uuid.NewString(), Valid: true}
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)`, metaDeviceID, dev.String); err != nil {
			return res, err
		}
	}
	res.Device, res.Tables = dev.String, len(reg.Order)
	node := replica.NodeID(dev.String)
	if _, err := tx.ExecContext(ctx, `UPDATE replica_clock SET device = ?, node = ?, schema_version = ? WHERE id = 1`, dev.String, node, SchemaVersion); err != nil {
		return res, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO replica_vector (device, seq) VALUES (?, 0)`, dev.String); err != nil {
		return res, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO replica_devices (device, name, self, role) VALUES (?, 'this device', 1, 'peer')
		ON CONFLICT(device) DO UPDATE SET self = 1`, dev.String); err != nil {
		return res, err
	}
	var before int64
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM replica_changes`).Scan(&before)
	if err := install(ctx, tx, reg); err != nil {
		return res, err
	}
	var after int64
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM replica_changes`).Scan(&after)
	res.Baseline = after - before
	for k, v := range map[string]string{metaEnabled: "1", metaSignature: reg.Signature()} {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)`, k, v); err != nil {
			return res, err
		}
	}
	return res, tx.Commit()
}

// Refresh keeps the triggers in step with the registry and the table columns. A build with a new
// registry line or a migration that adds a column changes the signature. It does nothing when sync
// is off. It also retries changes that were stored unapplied because an older build did not know
// their field.
func Refresh(ctx context.Context, db *sql.DB) error {
	if !IsEnabled(db) {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := ensureUUIDColumns(ctx, tx, DefaultRegistry()); err != nil {
		return err
	}
	reg, err := Describe(tx, DefaultRegistry())
	if err != nil {
		return err
	}
	var sig string
	_ = tx.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, metaSignature).Scan(&sig)
	if sig == reg.Signature() {
		return nil
	}
	if err := install(ctx, tx, reg); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE replica_clock SET schema_version = ? WHERE id = 1`, SchemaVersion); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)`, metaSignature, reg.Signature()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	a, err := Open(ctx, db, DefaultRegistry())
	if err != nil {
		return err
	}
	_, err = a.RetryUnknown(ctx)
	return err
}

// naturalNamespace is the fixed namespace of uuids derived from natural keys.
var naturalNamespace = uuid.MustParse("6f1c2a52-8d3e-4b57-9a0e-5c1d7e2b9f40")

// NaturalUUID is the uuid every device gives the row of a table with these natural key values.
func NaturalUUID(table string, values ...string) string {
	return uuid.NewSHA1(naturalNamespace, []byte(table+""+strings.ToLower(strings.Join(values, "")))).String()
}

// normalizeUUIDs gives rows of natural-key tables that have no change yet a uuid derived from the
// key, so devices that made the same row independently agree on its name.
func normalizeUUIDs(ctx context.Context, tx *sql.Tx, d *Described) error {
	if len(d.Natural) == 0 {
		return nil
	}
	cols := make([]string, len(d.Natural))
	for i, c := range d.Natural {
		cols[i] = "CAST(" + q(c) + " AS TEXT)"
	}
	rows, err := tx.QueryContext(ctx, `SELECT rowid, `+strings.Join(cols, ", ")+` FROM `+q(d.Name)+` AS t
		WHERE NOT EXISTS (SELECT 1 FROM replica_changes c WHERE c.collection = ? AND c.key = t.uuid)`, d.Name)
	if err != nil {
		return err
	}
	type item struct {
		rowid int64
		uuid  string
	}
	var items []item
	for rows.Next() {
		vals := make([]sql.NullString, len(cols))
		ptrs := make([]any, 0, len(cols)+1)
		var rowid int64
		ptrs = append(ptrs, &rowid)
		for i := range vals {
			ptrs = append(ptrs, &vals[i])
		}
		if err := rows.Scan(ptrs...); err != nil {
			_ = rows.Close()
			return err
		}
		strs := make([]string, len(vals))
		for i, v := range vals {
			strs[i] = v.String
		}
		items = append(items, item{rowid, NaturalUUID(d.Name, strs...)})
	}
	_ = rows.Close()
	for _, it := range items {
		if _, err := tx.ExecContext(ctx, `UPDATE `+q(d.Name)+` SET uuid = ? WHERE rowid = ? AND uuid IS NOT ?
			AND NOT EXISTS (SELECT 1 FROM `+q(d.Name)+` WHERE uuid = ?)`, it.uuid, it.rowid, it.uuid, it.uuid); err != nil {
			return err
		}
	}
	return nil
}

// ensureUUIDColumns adds and fills the uuid column of tables that did not have one.
func ensureUUIDColumns(ctx context.Context, tx *sql.Tx, tables []Table) error {
	for _, t := range tables {
		if !t.AddUUID {
			continue
		}
		rows, err := tx.QueryContext(ctx, `PRAGMA table_info(`+q(t.Name)+`)`)
		if err != nil {
			return err
		}
		has := false
		for rows.Next() {
			var cid, notnull, pk int
			var name, typ string
			var dflt sql.NullString
			if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
				_ = rows.Close()
				return err
			}
			has = has || name == "uuid"
		}
		_ = rows.Close()
		if has {
			continue
		}
		if _, err := tx.ExecContext(ctx, `ALTER TABLE `+q(t.Name)+` ADD COLUMN uuid TEXT`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE `+q(t.Name)+` SET uuid = `+genUUID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS `+q("idx_"+t.Name+"_uuid")+` ON `+q(t.Name)+`(uuid)`); err != nil {
			return err
		}
	}
	return nil
}
