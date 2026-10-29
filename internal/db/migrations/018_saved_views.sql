-- up

-- Saved views (owner request 2026-10-29, patterns/saved-views). A saved view is a named, stored query over
-- one object kind: filter, sort, group and columns as a neutral JSON query (patterns/saved-view-query),
-- never SQL. It is a sync-safe row like every other synced object: uuid, updated_at and a soft delete.
-- position orders the views, show_in_tabs says whether the view takes a tab and a number key, example
-- marks the three views that ship with the program so the user can tell them from their own.
-- The version a database gets is the position of the file, so never renumber.
CREATE TABLE saved_view (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid         TEXT NOT NULL UNIQUE,
    name         TEXT NOT NULL COLLATE NOCASE,
    kind         TEXT NOT NULL CHECK (kind IN ('person', 'organization', 'deal', 'task', 'note', 'tag')),
    query        TEXT NOT NULL DEFAULT '{}',
    position     INTEGER NOT NULL DEFAULT 0,
    show_in_tabs INTEGER NOT NULL DEFAULT 0 CHECK (show_in_tabs IN (0, 1)),
    example      INTEGER NOT NULL DEFAULT 0 CHECK (example IN (0, 1)),
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
    deleted_at   TEXT
);
-- The name is unique among live views only, so a deleted view does not block its name.
CREATE UNIQUE INDEX uq_saved_view_name ON saved_view(name) WHERE deleted_at IS NULL;
CREATE INDEX idx_saved_view_position ON saved_view(position) WHERE deleted_at IS NULL;

-- Three example views, off by default. Fixed uuids so every device agrees on them.
INSERT INTO saved_view (uuid, name, kind, query, position, show_in_tabs, example) VALUES
    ('b0000001-0000-4000-8000-000000000001', 'Overdue follow-up', 'person',
     '{"filter":[{"field":"last_contacted","op":"before","values":["90 days ago"]}],"sort":[{"field":"last_contacted","dir":"asc"}]}', 1, 0, 1),
    ('b0000001-0000-4000-8000-000000000002', 'Open deals by stage', 'deal',
     '{"filter":[{"field":"outcome","op":"is","values":["open"]}],"sort":[{"field":"value","dir":"desc"}],"group_by":"stage"}', 2, 0, 1),
    ('b0000001-0000-4000-8000-000000000003', 'Tasks due this week', 'task',
     '{"filter":[{"field":"status","op":"is","values":["open"]},{"field":"due","op":"in_next","values":["7"]}],"sort":[{"field":"due","dir":"asc"}],"group_by":"due","bucket":"day"}', 3, 0, 1);

-- down

DROP INDEX IF EXISTS idx_saved_view_position;
DROP INDEX IF EXISTS uq_saved_view_name;
DROP TABLE IF EXISTS saved_view;
