-- up

-- Device identity and sync groundwork ------------------------------------------------

CREATE TABLE IF NOT EXISTS meta (
    key TEXT PRIMARY KEY,
    value TEXT
);

INSERT OR IGNORE INTO meta (key, value) VALUES (
    'device_id',
    lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)), 2) || '-' ||
          substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2) || '-' || hex(randomblob(6)))
);

-- Permanent deletions are recorded here so a future sync can propagate them.
CREATE TABLE IF NOT EXISTS tombstones (
    table_name TEXT NOT NULL,
    uuid TEXT NOT NULL,
    deleted_at TEXT NOT NULL DEFAULT (datetime('now')),
    device_id TEXT,
    PRIMARY KEY (table_name, uuid)
);

-- Tags get a uuid and timestamps like every other synced entity.
ALTER TABLE tags ADD COLUMN uuid TEXT;
ALTER TABLE tags ADD COLUMN created_at TEXT;
ALTER TABLE tags ADD COLUMN updated_at TEXT;
UPDATE tags SET
    uuid = lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)), 2) || '-' ||
                 substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2) || '-' || hex(randomblob(6))),
    created_at = datetime('now'),
    updated_at = datetime('now');
CREATE UNIQUE INDEX IF NOT EXISTS idx_tags_uuid ON tags(uuid);

-- Note types ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS note_types (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL UNIQUE COLLATE NOCASE,
    counts_as_contact INTEGER NOT NULL DEFAULT 1,
    archived INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT OR IGNORE INTO note_types (uuid, name, counts_as_contact)
SELECT lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)), 2) || '-' ||
             substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2) || '-' || hex(randomblob(6))),
       name, counts
FROM (
    SELECT 'call' AS name, 1 AS counts UNION ALL
    SELECT 'email', 1 UNION ALL
    SELECT 'meeting', 1 UNION ALL
    SELECT 'note', 0 UNION ALL
    SELECT 'message', 1
);

-- interactions.type had a CHECK limited to the five seed values. SQLite cannot drop a
-- CHECK, so rebuild the table without it. Ids are preserved. interaction_people is
-- dropped first so no cascade can run, then restored.
DROP VIEW IF EXISTS keep_in_touch_status;

CREATE TEMP TABLE _ip_backup AS SELECT interaction_id, person_id FROM interaction_people;
DROP TABLE interaction_people;

ALTER TABLE interactions RENAME TO _interactions_old;
DROP TRIGGER IF EXISTS interactions_ai;
DROP TRIGGER IF EXISTS interactions_ad;
DROP TRIGGER IF EXISTS interactions_au;

CREATE TABLE interactions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,
    subject TEXT,
    content TEXT,
    direction TEXT CHECK(direction IN ('inbound', 'outbound')),
    occurred_at TEXT NOT NULL DEFAULT (datetime('now')),
    archived INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO interactions (id, uuid, type, subject, content, direction, occurred_at, archived, created_at, updated_at)
SELECT id, uuid, type, subject, content, direction, occurred_at, archived, created_at, updated_at
FROM _interactions_old;
DROP TABLE _interactions_old;

CREATE TABLE interaction_people (
    interaction_id INTEGER NOT NULL REFERENCES interactions(id) ON DELETE CASCADE,
    person_id INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    PRIMARY KEY (interaction_id, person_id)
);
INSERT INTO interaction_people (interaction_id, person_id) SELECT interaction_id, person_id FROM _ip_backup;
DROP TABLE _ip_backup;
CREATE INDEX IF NOT EXISTS idx_interaction_people_person ON interaction_people(person_id);
CREATE INDEX IF NOT EXISTS idx_interactions_type ON interactions(type);

CREATE TRIGGER interactions_ai AFTER INSERT ON interactions BEGIN
    INSERT INTO interactions_fts(rowid, subject, content)
    VALUES (new.id, new.subject, new.content);
END;

CREATE TRIGGER interactions_ad AFTER DELETE ON interactions BEGIN
    INSERT INTO interactions_fts(interactions_fts, rowid, subject, content)
    VALUES ('delete', old.id, old.subject, old.content);
END;

CREATE TRIGGER interactions_au AFTER UPDATE ON interactions BEGIN
    INSERT INTO interactions_fts(interactions_fts, rowid, subject, content)
    VALUES ('delete', old.id, old.subject, old.content);
    INSERT INTO interactions_fts(rowid, subject, content)
    VALUES (new.id, new.subject, new.content);
END;

INSERT INTO interactions_fts(interactions_fts) VALUES ('rebuild');

-- Keep-in-touch ------------------------------------------------------------------------

ALTER TABLE people ADD COLUMN keep_in_touch_set_at TEXT;

-- Existing cadences start counting from the upgrade, so imported contacts are not all overdue.
UPDATE people SET keep_in_touch_set_at = datetime('now') WHERE keep_in_touch IS NOT NULL AND keep_in_touch_set_at IS NULL;

-- last_seen is the later of the manual value and the latest interaction whose type counts
-- as contact. due_at is COALESCE(last_seen, keep_in_touch_set_at, created_at) plus the cadence.
CREATE VIEW keep_in_touch_status AS
SELECT
    p.id AS person_id,
    p.first_name,
    p.last_name,
    p.keep_in_touch,
    p.keep_in_touch_snoozed_until,
    ls.last_seen,
    CASE p.keep_in_touch
        WHEN '7 days'  THEN datetime(COALESCE(ls.last_seen, p.keep_in_touch_set_at, p.created_at), '+7 days')
        WHEN '14 days' THEN datetime(COALESCE(ls.last_seen, p.keep_in_touch_set_at, p.created_at), '+14 days')
        WHEN '1 mon'   THEN datetime(COALESCE(ls.last_seen, p.keep_in_touch_set_at, p.created_at), '+1 month')
        WHEN '42 days' THEN datetime(COALESCE(ls.last_seen, p.keep_in_touch_set_at, p.created_at), '+42 days')
        WHEN '3 mons'  THEN datetime(COALESCE(ls.last_seen, p.keep_in_touch_set_at, p.created_at), '+3 months')
        WHEN '6 mons'  THEN datetime(COALESCE(ls.last_seen, p.keep_in_touch_set_at, p.created_at), '+6 months')
        WHEN '1 year'  THEN datetime(COALESCE(ls.last_seen, p.keep_in_touch_set_at, p.created_at), '+1 year')
    END AS due_at
FROM people p
LEFT JOIN (
    SELECT person_id,
           CASE
               WHEN manual IS NULL THEN derived
               WHEN derived IS NULL THEN manual
               ELSE MAX(manual, derived)
           END AS last_seen
    FROM (
        SELECT p2.id AS person_id,
               p2.last_contacted_at AS manual,
               MAX(i.occurred_at) AS derived
        FROM people p2
        LEFT JOIN interaction_people ip ON ip.person_id = p2.id
        LEFT JOIN interactions i ON i.id = ip.interaction_id AND i.archived = 0
             AND COALESCE((SELECT nt.counts_as_contact FROM note_types nt WHERE nt.name = i.type), 1) = 1
        GROUP BY p2.id
    )
) ls ON ls.person_id = p.id
WHERE p.archived = 0
  AND p.keep_in_touch IS NOT NULL
  AND p.keep_in_touch != 'never';

-- down

DROP VIEW IF EXISTS keep_in_touch_status;

ALTER TABLE people DROP COLUMN keep_in_touch_set_at;

-- Restore the type CHECK. Custom types collapse to note so the CHECK holds.
CREATE TEMP TABLE _ip_backup AS SELECT interaction_id, person_id FROM interaction_people;
DROP TABLE interaction_people;
ALTER TABLE interactions RENAME TO _interactions_old;
DROP TRIGGER IF EXISTS interactions_ai;
DROP TRIGGER IF EXISTS interactions_ad;
DROP TRIGGER IF EXISTS interactions_au;
DROP INDEX IF EXISTS idx_interactions_type;
CREATE TABLE interactions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL CHECK(type IN ('call', 'email', 'meeting', 'note', 'message')),
    subject TEXT,
    content TEXT,
    direction TEXT CHECK(direction IN ('inbound', 'outbound')),
    occurred_at TEXT NOT NULL DEFAULT (datetime('now')),
    archived INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO interactions (id, uuid, type, subject, content, direction, occurred_at, archived, created_at, updated_at)
SELECT id, uuid,
       CASE WHEN type IN ('call', 'email', 'meeting', 'note', 'message') THEN type ELSE 'note' END,
       subject, content, direction, occurred_at, archived, created_at, updated_at
FROM _interactions_old;
DROP TABLE _interactions_old;
CREATE TABLE interaction_people (
    interaction_id INTEGER NOT NULL REFERENCES interactions(id) ON DELETE CASCADE,
    person_id INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    PRIMARY KEY (interaction_id, person_id)
);
INSERT INTO interaction_people (interaction_id, person_id) SELECT interaction_id, person_id FROM _ip_backup;
DROP TABLE _ip_backup;
CREATE INDEX IF NOT EXISTS idx_interaction_people_person ON interaction_people(person_id);
CREATE TRIGGER interactions_ai AFTER INSERT ON interactions BEGIN
    INSERT INTO interactions_fts(rowid, subject, content)
    VALUES (new.id, new.subject, new.content);
END;
CREATE TRIGGER interactions_ad AFTER DELETE ON interactions BEGIN
    INSERT INTO interactions_fts(interactions_fts, rowid, subject, content)
    VALUES ('delete', old.id, old.subject, old.content);
END;
CREATE TRIGGER interactions_au AFTER UPDATE ON interactions BEGIN
    INSERT INTO interactions_fts(interactions_fts, rowid, subject, content)
    VALUES ('delete', old.id, old.subject, old.content);
    INSERT INTO interactions_fts(rowid, subject, content)
    VALUES (new.id, new.subject, new.content);
END;
INSERT INTO interactions_fts(interactions_fts) VALUES ('rebuild');

DROP TABLE IF EXISTS note_types;
DROP INDEX IF EXISTS idx_tags_uuid;
ALTER TABLE tags DROP COLUMN updated_at;
ALTER TABLE tags DROP COLUMN created_at;
ALTER TABLE tags DROP COLUMN uuid;
DROP TABLE IF EXISTS tombstones;
DROP TABLE IF EXISTS meta;

-- Restore the 002 view.
CREATE VIEW keep_in_touch_status AS
SELECT
    p.id AS person_id,
    p.first_name,
    p.last_name,
    p.keep_in_touch,
    p.keep_in_touch_snoozed_until,
    ls.last_seen,
    CASE p.keep_in_touch
        WHEN '7 days'  THEN datetime(COALESCE(ls.last_seen, p.created_at), '+7 days')
        WHEN '14 days' THEN datetime(COALESCE(ls.last_seen, p.created_at), '+14 days')
        WHEN '1 mon'   THEN datetime(COALESCE(ls.last_seen, p.created_at), '+1 month')
        WHEN '42 days' THEN datetime(COALESCE(ls.last_seen, p.created_at), '+42 days')
        WHEN '3 mons'  THEN datetime(COALESCE(ls.last_seen, p.created_at), '+3 months')
        WHEN '6 mons'  THEN datetime(COALESCE(ls.last_seen, p.created_at), '+6 months')
        WHEN '1 year'  THEN datetime(COALESCE(ls.last_seen, p.created_at), '+1 year')
    END AS due_at
FROM people p
LEFT JOIN (
    SELECT person_id,
           CASE
               WHEN manual IS NULL THEN derived
               WHEN derived IS NULL THEN manual
               ELSE MAX(manual, derived)
           END AS last_seen
    FROM (
        SELECT p2.id AS person_id,
               p2.last_contacted_at AS manual,
               MAX(i.occurred_at) AS derived
        FROM people p2
        LEFT JOIN interaction_people ip ON ip.person_id = p2.id
        LEFT JOIN interactions i ON i.id = ip.interaction_id AND i.type != 'note' AND i.archived = 0
        GROUP BY p2.id
    )
) ls ON ls.person_id = p.id
WHERE p.archived = 0
  AND p.keep_in_touch IS NOT NULL
  AND p.keep_in_touch != 'never';
