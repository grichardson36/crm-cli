-- up

-- External ids (H01) ------------------------------------------------------------------
-- Remembers which external record a local row came from so a re-sync updates instead of
-- duplicating. local_id is the integer id of the local row in the table named by entity.

CREATE TABLE IF NOT EXISTS external_ids (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    entity TEXT NOT NULL,
    local_id INTEGER NOT NULL,
    local_uuid TEXT NOT NULL,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    etag TEXT,
    last_seen_at TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(source, entity, external_id),
    UNIQUE(source, entity, local_id)
);
CREATE INDEX IF NOT EXISTS idx_external_ids_local ON external_ids(entity, local_id);

-- Backfill the Dex importer's dex_id custom fields. The custom field rows stay.
INSERT OR IGNORE INTO external_ids (uuid, entity, local_id, local_uuid, source, external_id, last_seen_at)
SELECT lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)), 2) || '-' ||
             substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2) || '-' || hex(randomblob(6))),
       'person', p.id, p.uuid, 'dex', cf.field_value, datetime('now')
FROM custom_fields cf
JOIN people p ON p.id = cf.entity_id
WHERE cf.entity_type = 'person' AND cf.field_name = 'dex_id' AND TRIM(cf.field_value) != '';

-- Sync runner (H02) -------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS sync_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    source TEXT NOT NULL,
    started_at TEXT NOT NULL DEFAULT (datetime('now')),
    finished_at TEXT,
    status TEXT NOT NULL DEFAULT 'running',
    created INTEGER NOT NULL DEFAULT 0,
    updated INTEGER NOT NULL DEFAULT 0,
    skipped INTEGER NOT NULL DEFAULT 0,
    errors INTEGER NOT NULL DEFAULT 0,
    note TEXT
);
CREATE INDEX IF NOT EXISTS idx_sync_runs_source ON sync_runs(source, id);

CREATE TABLE IF NOT EXISTS sync_cursors (
    source TEXT PRIMARY KEY,
    cursor TEXT,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Records that matched nobody wait here for accept, attach or ignore.
CREATE TABLE IF NOT EXISTS sync_inbox (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    payload TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_sync_inbox_status ON sync_inbox(status, id);
CREATE INDEX IF NOT EXISTS idx_sync_inbox_source ON sync_inbox(source, external_id);

-- Per-field change tracking (groundwork for CardDAV conflict handling) ----------------

CREATE TABLE IF NOT EXISTS field_versions (
    entity TEXT NOT NULL,
    local_id INTEGER NOT NULL,
    field TEXT NOT NULL,
    changed_at TEXT NOT NULL,
    device_id TEXT,
    PRIMARY KEY (entity, local_id, field)
) WITHOUT ROWID;

-- down

DROP TABLE IF EXISTS field_versions;
DROP TABLE IF EXISTS sync_inbox;
DROP TABLE IF EXISTS sync_cursors;
DROP TABLE IF EXISTS sync_runs;
DROP TABLE IF EXISTS external_ids;
