-- up

-- Verbs and undo (owner decisions 2026-10-27). The version a database gets is the position of the
-- file, so never renumber.
--
-- 1. taggings become sync-safe link rows like every other link table: uuid, created_at, updated_at
--    and deleted_at. Untagging soft deletes the row, tagging again revives it with its old row. A tag
--    is only a name on objects, so tags get no deleted_at: deleting a tag soft deletes its taggings
--    and a tag with no live tagging drops out of tag lists. Existing rows keep their ids. The default
--    uuid keeps older INSERTs that name only tag_id, entity_type and entity_id working.
CREATE TABLE taggings_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL DEFAULT (lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)), 2) || '-' ||
          substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2) || '-' || hex(randomblob(6)))),
    tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL CHECK(entity_type IN ('person', 'organization', 'deal', 'interaction', 'task')),
    entity_id INTEGER NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    deleted_at TEXT
);

INSERT INTO taggings_new (id, tag_id, entity_type, entity_id)
    SELECT id, tag_id, entity_type, entity_id FROM taggings;

DROP TABLE taggings;
ALTER TABLE taggings_new RENAME TO taggings;

CREATE UNIQUE INDEX uq_taggings_uuid ON taggings(uuid);
CREATE UNIQUE INDEX uq_taggings_live ON taggings(tag_id, entity_type, entity_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_taggings_entity ON taggings(entity_type, entity_id);

-- 2. Merge log: one row per person or organization merge, kept across sessions so
--    `crm merge undo <id>` works later. edits is a JSON list of row changes (table, id, the columns
--    that changed with their value before and after, or the whole row for an insert), summary holds
--    the counts of what moved. status goes merged -> undone -> merged (redo).
CREATE TABLE merge_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL CHECK(kind IN ('person', 'organization')),
    survivor_id INTEGER NOT NULL,
    survivor_name TEXT NOT NULL DEFAULT '',
    loser_ids TEXT NOT NULL DEFAULT '[]',
    loser_names TEXT NOT NULL DEFAULT '[]',
    summary TEXT NOT NULL DEFAULT '{}',
    edits TEXT NOT NULL DEFAULT '[]',
    status TEXT NOT NULL DEFAULT 'merged' CHECK(status IN ('merged', 'undone')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    undone_at TEXT
);
CREATE INDEX idx_merge_log_survivor ON merge_log(kind, survivor_id);

-- 3. "Not a duplicate" for organization merge candidates, stored as data so it can sync later.
--    Dismissing again revives the row, un-dismissing soft deletes it. org_a < org_b.
CREATE TABLE org_dupe_dismissals (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    org_a INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    org_b INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    dismissed_at TEXT NOT NULL DEFAULT (datetime('now')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    deleted_at TEXT,
    UNIQUE (org_a, org_b),
    CHECK (org_a < org_b)
);
CREATE INDEX idx_org_dupe_dismissals_b ON org_dupe_dismissals(org_b);

-- down

DROP INDEX IF EXISTS idx_org_dupe_dismissals_b;
DROP TABLE IF EXISTS org_dupe_dismissals;
DROP INDEX IF EXISTS idx_merge_log_survivor;
DROP TABLE IF EXISTS merge_log;
DELETE FROM taggings WHERE deleted_at IS NOT NULL;
CREATE TABLE taggings_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL CHECK(entity_type IN ('person', 'organization', 'deal', 'interaction', 'task')),
    entity_id INTEGER NOT NULL,
    UNIQUE(tag_id, entity_type, entity_id)
);
INSERT INTO taggings_old (id, tag_id, entity_type, entity_id) SELECT id, tag_id, entity_type, entity_id FROM taggings;
DROP TABLE taggings;
ALTER TABLE taggings_old RENAME TO taggings;
CREATE INDEX IF NOT EXISTS idx_taggings_entity ON taggings(entity_type, entity_id);
