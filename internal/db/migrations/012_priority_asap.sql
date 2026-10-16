-- up

-- Task priority gains ASAP, as in Motion (ASAP, High, Medium, Low). The priority CHECK lists the
-- allowed values, so the tasks table is rebuilt. Rows that point at tasks are saved and restored
-- around the rebuild because dropping the table would cascade into them.
CREATE TEMP TABLE _task_people_backup AS SELECT task_id, person_id FROM task_people;
CREATE TEMP TABLE _task_chunks_backup AS SELECT task_id, start_utc, end_utc FROM task_chunks;
DROP TABLE task_people;
DROP TABLE task_chunks;
ALTER TABLE tasks RENAME TO _tasks_old;
DROP INDEX IF EXISTS idx_tasks_person;
DROP INDEX IF EXISTS idx_tasks_deal;
DROP INDEX IF EXISTS idx_tasks_due;

CREATE TABLE tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    description TEXT,
    person_id INTEGER REFERENCES people(id),
    deal_id INTEGER REFERENCES deals(id),
    due_at TEXT,
    priority TEXT NOT NULL DEFAULT 'medium' CHECK(priority IN ('low', 'medium', 'high', 'asap')),
    completed INTEGER NOT NULL DEFAULT 0,
    completed_at TEXT,
    archived INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    recurrence TEXT,
    snoozed_until TEXT,
    duration_min INTEGER CHECK(duration_min IS NULL OR duration_min > 0),
    earliest_start TEXT,
    deadline_at TEXT,
    deadline_hard INTEGER NOT NULL DEFAULT 0,
    min_chunk_min INTEGER CHECK(min_chunk_min IS NULL OR min_chunk_min > 0),
    auto_schedule INTEGER NOT NULL DEFAULT 0,
    scheduled_start TEXT,
    scheduled_locked INTEGER NOT NULL DEFAULT 0,
    schedule_name TEXT NOT NULL DEFAULT 'work'
);
INSERT INTO tasks (id, uuid, title, description, person_id, deal_id, due_at, priority, completed, completed_at, archived,
    created_at, updated_at, recurrence, snoozed_until, duration_min, earliest_start, deadline_at, deadline_hard, min_chunk_min,
    auto_schedule, scheduled_start, scheduled_locked, schedule_name)
SELECT id, uuid, title, description, person_id, deal_id, due_at, priority, completed, completed_at, archived,
    created_at, updated_at, recurrence, snoozed_until, duration_min, earliest_start, deadline_at, deadline_hard, min_chunk_min,
    auto_schedule, scheduled_start, scheduled_locked, schedule_name
FROM _tasks_old;
DROP TABLE _tasks_old;
CREATE INDEX idx_tasks_person ON tasks(person_id) WHERE archived = 0;
CREATE INDEX idx_tasks_deal ON tasks(deal_id) WHERE archived = 0;
CREATE INDEX idx_tasks_due ON tasks(due_at) WHERE archived = 0 AND completed = 0;

CREATE TABLE task_people (
    task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    person_id INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    PRIMARY KEY (task_id, person_id)
);
CREATE INDEX idx_task_people_person ON task_people(person_id);
INSERT INTO task_people (task_id, person_id) SELECT task_id, person_id FROM _task_people_backup;
CREATE TABLE task_chunks (
    task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    start_utc TEXT NOT NULL,
    end_utc TEXT NOT NULL,
    PRIMARY KEY (task_id, start_utc)
) WITHOUT ROWID;
INSERT INTO task_chunks (task_id, start_utc, end_utc) SELECT task_id, start_utc, end_utc FROM _task_chunks_backup;
DROP TABLE _task_people_backup;
DROP TABLE _task_chunks_backup;

-- down

UPDATE tasks SET priority = 'high' WHERE priority = 'asap';
CREATE TEMP TABLE _task_people_backup AS SELECT task_id, person_id FROM task_people;
CREATE TEMP TABLE _task_chunks_backup AS SELECT task_id, start_utc, end_utc FROM task_chunks;
DROP TABLE task_people;
DROP TABLE task_chunks;
ALTER TABLE tasks RENAME TO _tasks_old;
DROP INDEX IF EXISTS idx_tasks_person;
DROP INDEX IF EXISTS idx_tasks_deal;
DROP INDEX IF EXISTS idx_tasks_due;
CREATE TABLE tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    description TEXT,
    person_id INTEGER REFERENCES people(id),
    deal_id INTEGER REFERENCES deals(id),
    due_at TEXT,
    priority TEXT NOT NULL DEFAULT 'medium' CHECK(priority IN ('low', 'medium', 'high')),
    completed INTEGER NOT NULL DEFAULT 0,
    completed_at TEXT,
    archived INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    recurrence TEXT,
    snoozed_until TEXT,
    duration_min INTEGER CHECK(duration_min IS NULL OR duration_min > 0),
    earliest_start TEXT,
    deadline_at TEXT,
    deadline_hard INTEGER NOT NULL DEFAULT 0,
    min_chunk_min INTEGER CHECK(min_chunk_min IS NULL OR min_chunk_min > 0),
    auto_schedule INTEGER NOT NULL DEFAULT 0,
    scheduled_start TEXT,
    scheduled_locked INTEGER NOT NULL DEFAULT 0,
    schedule_name TEXT NOT NULL DEFAULT 'work'
);
INSERT INTO tasks SELECT * FROM _tasks_old;
DROP TABLE _tasks_old;
CREATE INDEX idx_tasks_person ON tasks(person_id) WHERE archived = 0;
CREATE INDEX idx_tasks_deal ON tasks(deal_id) WHERE archived = 0;
CREATE INDEX idx_tasks_due ON tasks(due_at) WHERE archived = 0 AND completed = 0;
CREATE TABLE task_people (
    task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    person_id INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    PRIMARY KEY (task_id, person_id)
);
CREATE INDEX idx_task_people_person ON task_people(person_id);
INSERT INTO task_people (task_id, person_id) SELECT task_id, person_id FROM _task_people_backup;
CREATE TABLE task_chunks (
    task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    start_utc TEXT NOT NULL,
    end_utc TEXT NOT NULL,
    PRIMARY KEY (task_id, start_utc)
) WITHOUT ROWID;
INSERT INTO task_chunks (task_id, start_utc, end_utc) SELECT task_id, start_utc, end_utc FROM _task_chunks_backup;
DROP TABLE _task_people_backup;
DROP TABLE _task_chunks_backup;
