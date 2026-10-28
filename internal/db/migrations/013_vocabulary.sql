-- up

-- User-managed vocabularies: one table, one row per value, keyed by kind (deal_role today, deal
-- stages and note types later). position is the pick order. archived rows are removed values: they
-- stay so the rows that still hold the text keep showing it, and so a removed name can come back.
-- The version a database gets is the position of the file, so never renumber.
CREATE TABLE vocabulary (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid       TEXT NOT NULL UNIQUE,
    kind       TEXT NOT NULL,
    name       TEXT NOT NULL COLLATE NOCASE,
    position   INTEGER NOT NULL DEFAULT 0,
    archived   INTEGER NOT NULL DEFAULT 0 CHECK (archived IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (kind, name)
);
CREATE INDEX idx_vocabulary_kind ON vocabulary(kind, archived, position);

-- Seeded deal roles: the sales roles of the owner's HubSpot export (champion, economic buyer,
-- signer, billing contact) and the usual buying-committee roles.
INSERT INTO vocabulary (uuid, kind, name, position)
SELECT lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)), 2) || '-' ||
             substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2) || '-' || hex(randomblob(6))),
       'deal_role', name, pos
FROM (
    SELECT 'champion' AS name, 1 AS pos UNION ALL
    SELECT 'decision maker', 2 UNION ALL
    SELECT 'economic buyer', 3 UNION ALL
    SELECT 'influencer', 4 UNION ALL
    SELECT 'blocker', 5 UNION ALL
    SELECT 'end user', 6 UNION ALL
    SELECT 'signer', 7 UNION ALL
    SELECT 'billing contact', 8
);

-- down

DROP TABLE vocabulary;
