-- up

-- Tasks can carry tags. taggings had a CHECK that listed the taggable entities, and SQLite cannot
-- change a CHECK in place, so the table is rebuilt with 'task' in the list. Rows, ids and the
-- unique key are kept. The number 009 leaves 007 and 008 free for other branches; the version a
-- database gets is the position of the file, so never renumber.
CREATE TABLE taggings_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL CHECK(entity_type IN ('person', 'organization', 'deal', 'interaction', 'task')),
    entity_id INTEGER NOT NULL,
    UNIQUE(tag_id, entity_type, entity_id)
);

INSERT INTO taggings_new (id, tag_id, entity_type, entity_id)
    SELECT id, tag_id, entity_type, entity_id FROM taggings;

DROP TABLE taggings;
ALTER TABLE taggings_new RENAME TO taggings;

CREATE INDEX IF NOT EXISTS idx_taggings_entity ON taggings(entity_type, entity_id);

-- down

-- Not reversible without losing task tags: delete them, then rebuild with the old list.
DELETE FROM taggings WHERE entity_type = 'task';
CREATE TABLE taggings_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL CHECK(entity_type IN ('person', 'organization', 'deal', 'interaction')),
    entity_id INTEGER NOT NULL,
    UNIQUE(tag_id, entity_type, entity_id)
);
INSERT INTO taggings_old (id, tag_id, entity_type, entity_id) SELECT id, tag_id, entity_type, entity_id FROM taggings;
DROP TABLE taggings;
ALTER TABLE taggings_old RENAME TO taggings;
CREATE INDEX IF NOT EXISTS idx_taggings_entity ON taggings(entity_type, entity_id);
