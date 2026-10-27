-- up

-- Company strings that must never become an organization (N/A, none, self-employed ...). The built-in
-- defaults live in the program. This table holds the user's changes to them: mode 'add' ignores one
-- more string, mode 'remove' turns a built-in off. key is the normalized form of the text. uuid and
-- updated_at are here so the list can sync between devices (it replaces the device-local meta keys).
-- The version a database gets is the position of the file, so never renumber.
CREATE TABLE company_ignore (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid       TEXT NOT NULL UNIQUE,
    key        TEXT NOT NULL UNIQUE,
    text       TEXT NOT NULL,
    mode       TEXT NOT NULL DEFAULT 'add' CHECK (mode IN ('add', 'remove')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- down

DROP TABLE company_ignore;
