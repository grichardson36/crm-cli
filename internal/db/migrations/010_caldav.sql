-- up

-- What the CalDAV adapter knows about each remote resource: its path, the ETag it last saw, and
-- the state of the event when both sides last agreed. A later run compares against this base to
-- tell which side changed.
CREATE TABLE IF NOT EXISTS caldav_resources (
    source TEXT NOT NULL,
    href TEXT NOT NULL,
    uid TEXT NOT NULL,
    etag TEXT,
    base_hash TEXT NOT NULL DEFAULT '',
    base_json TEXT NOT NULL DEFAULT '{}',
    synced_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (source, href)
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_caldav_resources_uid ON caldav_resources(source, uid);

-- down

DROP INDEX IF EXISTS idx_caldav_resources_uid;
DROP TABLE IF EXISTS caldav_resources;
