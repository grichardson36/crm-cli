-- up

-- Device-to-device sync (patterns/sync-contract.md). Every statement can run twice. Nothing here is used until `crm peers init`
-- turns sync on. The version a database gets is the position of the file, so never renumber.

-- One row. phys and cnt are the hybrid logical clock. device is this database's sync identity and
-- node its 16 hex characters. applying is 1 while remote changes are applied, so the capture
-- triggers stay quiet (begin_remote / end_remote). schema_version goes into every change written.
CREATE TABLE IF NOT EXISTS replica_clock (
    id             INTEGER PRIMARY KEY CHECK (id = 1),
    phys           TEXT NOT NULL DEFAULT '0000-01-01T00:00:00.000Z',
    cnt            INTEGER NOT NULL DEFAULT 0,
    device         TEXT NOT NULL DEFAULT '',
    node           TEXT NOT NULL DEFAULT '',
    applying       INTEGER NOT NULL DEFAULT 0,
    schema_version INTEGER NOT NULL DEFAULT 0
);
INSERT OR IGNORE INTO replica_clock (id) VALUES (1);

-- The version vector: the highest sequence number seen from each device. The row of this device is
-- also the counter for its own changes.
CREATE TABLE IF NOT EXISTS replica_vector (
    device TEXT PRIMARY KEY,
    seq    INTEGER NOT NULL
) WITHOUT ROWID;

-- The latest change of each device for each field of each record, and for the record itself
-- (field is empty for delete and restore). This is the per-field state, the log that peers pull,
-- and the place unknown fields wait (status unknown). seen is the writer's version vector, only set
-- for deletes and keep-both fields.
CREATE TABLE IF NOT EXISTS replica_changes (
    collection     TEXT NOT NULL,
    key            TEXT NOT NULL,
    field          TEXT NOT NULL,
    device         TEXT NOT NULL,
    seq            INTEGER NOT NULL,
    hlc            TEXT NOT NULL,
    op             TEXT NOT NULL CHECK (op IN ('set', 'unset', 'delete', 'restore')),
    value          TEXT,
    schema_version INTEGER NOT NULL DEFAULT 0,
    tx             TEXT,
    seen           TEXT,
    status         TEXT NOT NULL DEFAULT 'applied' CHECK (status IN ('applied', 'unknown')),
    PRIMARY KEY (collection, key, field, device)
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_replica_changes_device_seq ON replica_changes(device, seq);
CREATE INDEX IF NOT EXISTS idx_replica_changes_unknown ON replica_changes(status) WHERE status <> 'applied';

-- Every device this database knows: itself, the peers it connects to (url and token), and the
-- devices enrolled at it. vector and head are what the device last reported, used to decide when a
-- tombstone may be purged.
CREATE TABLE IF NOT EXISTS replica_devices (
    device       TEXT PRIMARY KEY,
    name         TEXT NOT NULL DEFAULT '',
    self         INTEGER NOT NULL DEFAULT 0 CHECK (self IN (0, 1)),
    url          TEXT,
    token        TEXT,
    role         TEXT NOT NULL DEFAULT 'peer',
    enrolled_at  TEXT NOT NULL DEFAULT (datetime('now')),
    retired_at   TEXT,
    vector       TEXT NOT NULL DEFAULT '{}',
    head         INTEGER NOT NULL DEFAULT 0,
    last_seen_at TEXT,
    last_pull_at TEXT,
    last_push_at TEXT,
    last_error   TEXT
) WITHOUT ROWID;

-- Tokens a relay accepts. Only the hash is stored. An enroll token is used once to add a device.
CREATE TABLE IF NOT EXISTS replica_tokens (
    hash       TEXT PRIMARY KEY,
    kind       TEXT NOT NULL CHECK (kind IN ('enroll', 'device')),
    label      TEXT NOT NULL DEFAULT '',
    device     TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT,
    used_at    TEXT
) WITHOUT ROWID;

-- Delete versus edit waits here. Nothing is lost: the record stays visible until a person decides.
CREATE TABLE IF NOT EXISTS replica_review (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    collection  TEXT NOT NULL,
    key         TEXT NOT NULL,
    detail      TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'kept', 'deleted', 'auto')),
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    resolved_at TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_replica_review_open ON replica_review(collection, key) WHERE status = 'open';

-- Records that could not be written to their table yet (a field or a referenced record is missing,
-- or a table rule refused them). They are retried after every apply.
CREATE TABLE IF NOT EXISTS replica_pending (
    collection TEXT NOT NULL,
    key        TEXT NOT NULL,
    reason     TEXT NOT NULL,
    since      TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (collection, key)
) WITHOUT ROWID;

-- Contact methods and custom fields have no uuid. Turning sync on adds the column (crm peers init),
-- because SQLite cannot add a column twice and this file may run again on a stepped-back database.

-- down

DROP TABLE IF EXISTS replica_pending;
DROP TABLE IF EXISTS replica_review;
DROP TABLE IF EXISTS replica_tokens;
DROP TABLE IF EXISTS replica_devices;
DROP TABLE IF EXISTS replica_changes;
DROP TABLE IF EXISTS replica_vector;
DROP TABLE IF EXISTS replica_clock;
