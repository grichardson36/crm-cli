-- up

-- Blocks of a task the planner split into chunks. scheduled_start on the task is the first block.
-- Replaced wholesale whenever a plan is applied.
CREATE TABLE IF NOT EXISTS task_chunks (
    task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    start_utc TEXT NOT NULL,
    end_utc TEXT NOT NULL,
    PRIMARY KEY (task_id, start_utc)
) WITHOUT ROWID;

-- One row per applied plan: what each task had before, so the plan can be undone.
CREATE TABLE IF NOT EXISTS plan_journal (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    note TEXT,
    entries_json TEXT NOT NULL,
    undone INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- down

DROP TABLE IF EXISTS plan_journal;
DROP TABLE IF EXISTS task_chunks;
