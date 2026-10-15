-- up

-- Task scheduling fields (J05). The existing due_at stays the soft target, priority stays
-- low/medium/high. A task with auto_schedule = 1 and no duration_min is reported, never placed.
ALTER TABLE tasks ADD COLUMN duration_min INTEGER CHECK(duration_min IS NULL OR duration_min > 0);
ALTER TABLE tasks ADD COLUMN earliest_start TEXT;
ALTER TABLE tasks ADD COLUMN deadline_at TEXT;
ALTER TABLE tasks ADD COLUMN deadline_hard INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN min_chunk_min INTEGER CHECK(min_chunk_min IS NULL OR min_chunk_min > 0);
ALTER TABLE tasks ADD COLUMN auto_schedule INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN scheduled_start TEXT;
ALTER TABLE tasks ADD COLUMN scheduled_locked INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN schedule_name TEXT NOT NULL DEFAULT 'work';

-- Working hours (Q5). Wall clock in the rule's zone, converted per date so daylight saving works.
-- weekly_json: {"mon":[["09:00","17:00"]], ...}. A day that is missing has no hours.
CREATE TABLE IF NOT EXISTS availability_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL UNIQUE,
    tz TEXT NOT NULL,
    weekly_json TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    archived INTEGER NOT NULL DEFAULT 0
);

-- Date-specific hours: an empty list is a day off.
CREATE TABLE IF NOT EXISTS availability_overrides (
    rule_id INTEGER NOT NULL REFERENCES availability_rules(id) ON DELETE CASCADE,
    date TEXT NOT NULL,
    intervals_json TEXT NOT NULL,
    PRIMARY KEY (rule_id, date)
) WITHOUT ROWID;

-- down

DROP TABLE IF EXISTS availability_overrides;
DROP TABLE IF EXISTS availability_rules;
ALTER TABLE tasks DROP COLUMN schedule_name;
ALTER TABLE tasks DROP COLUMN scheduled_locked;
ALTER TABLE tasks DROP COLUMN scheduled_start;
ALTER TABLE tasks DROP COLUMN auto_schedule;
ALTER TABLE tasks DROP COLUMN min_chunk_min;
ALTER TABLE tasks DROP COLUMN deadline_hard;
ALTER TABLE tasks DROP COLUMN deadline_at;
ALTER TABLE tasks DROP COLUMN earliest_start;
ALTER TABLE tasks DROP COLUMN duration_min;
