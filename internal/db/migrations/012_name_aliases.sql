-- up

-- The user's changes to the built-in given-name aliases (internal/alias). mode 'add' makes alias a
-- nickname of canonical, mode 'remove' turns a built-in alias off. alias is one lower-case word.
-- uuid and updated_at are here so the list can sync between devices.
-- The version a database gets is the position of the file, so never renumber.
CREATE TABLE name_aliases (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid       TEXT NOT NULL UNIQUE,
    alias      TEXT NOT NULL UNIQUE,
    canonical  TEXT NOT NULL,
    mode       TEXT NOT NULL DEFAULT 'add' CHECK (mode IN ('add', 'remove')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- down

DROP TABLE name_aliases;
