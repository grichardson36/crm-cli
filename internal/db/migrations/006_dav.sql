-- up

-- CardDAV server state (H06) -----------------------------------------------------------
-- One row per person the phone has seen. base_json is the contact exactly as the device last
-- held it (the base of the three-way merge). extra_vcard keeps the vCard properties this CRM
-- does not model, so a round trip through the phone does not lose them. etag is the version
-- the device last acknowledged.
CREATE TABLE IF NOT EXISTS dav_state (
    person_id INTEGER PRIMARY KEY REFERENCES people(id) ON DELETE CASCADE,
    base_json TEXT,
    etag TEXT,
    extra_vcard TEXT,
    synced_at TEXT
);

-- Every field where the phone and the CRM both changed, and who won. Link rows (field '_link')
-- record a phone contact that was attached to an existing person instead of creating a new one.
CREATE TABLE IF NOT EXISTS sync_conflicts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    person_id INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    field TEXT NOT NULL,
    crm_value TEXT,
    device_value TEXT,
    winner TEXT NOT NULL,
    resolved_at TEXT,
    undone INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_sync_conflicts_person ON sync_conflicts(person_id);
CREATE INDEX IF NOT EXISTS idx_sync_conflicts_undone ON sync_conflicts(undone, id);

-- down

DROP INDEX IF EXISTS idx_sync_conflicts_undone;
DROP INDEX IF EXISTS idx_sync_conflicts_person;
DROP TABLE IF EXISTS sync_conflicts;
DROP TABLE IF EXISTS dav_state;
