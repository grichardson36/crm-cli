-- up

-- Pairs of people the user marked "not duplicates". person_a < person_b.
-- Undismissing sets archived = 1 (soft delete) so the row can sync to other devices.
CREATE TABLE IF NOT EXISTS dupe_dismissals (
    person_a INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    person_b INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    uuid TEXT NOT NULL UNIQUE,
    dismissed_at TEXT NOT NULL DEFAULT (datetime('now')),
    archived INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (person_a, person_b),
    CHECK (person_a < person_b)
);
CREATE INDEX IF NOT EXISTS idx_dupe_dismissals_b ON dupe_dismissals(person_b);

-- A merged person is archived and points at the survivor.
ALTER TABLE people ADD COLUMN merged_into INTEGER REFERENCES people(id);
CREATE INDEX IF NOT EXISTS idx_people_merged_into ON people(merged_into) WHERE merged_into IS NOT NULL;

-- down

DROP INDEX IF EXISTS idx_people_merged_into;
ALTER TABLE people DROP COLUMN merged_into;
DROP INDEX IF EXISTS idx_dupe_dismissals_b;
DROP TABLE IF EXISTS dupe_dismissals;
