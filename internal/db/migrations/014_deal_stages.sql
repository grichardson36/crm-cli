-- foreign_keys: off
-- up

-- Customizable deal stages (owner decision 2026-10-29). The stages move onto the vocabulary table
-- (migration 013) as kind deal_stage. Each stage has an outcome: open, won or lost. The outcome decides
-- what closes a deal (closed_at, open deal counts, won and lost reports), the names do not.
-- The outcome column is only meaningful for the deal_stage kind and is open for every other kind.
-- The version a database gets is the position of the file, so never renumber.
ALTER TABLE vocabulary ADD COLUMN outcome TEXT NOT NULL DEFAULT 'open' CHECK (outcome IN ('open', 'won', 'lost'));

-- The six stages deals have always had, in order. Existing deals keep their stage text, which is one
-- of these names.
INSERT INTO vocabulary (uuid, kind, name, position, outcome)
SELECT lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)), 2) || '-' ||
             substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2) || '-' || hex(randomblob(6))),
       'deal_stage', name, pos, outcome
FROM (
    SELECT 'lead' AS name, 1 AS pos, 'open' AS outcome UNION ALL
    SELECT 'prospect', 2, 'open' UNION ALL
    SELECT 'proposal', 3, 'open' UNION ALL
    SELECT 'negotiation', 4, 'open' UNION ALL
    SELECT 'won', 5, 'won' UNION ALL
    SELECT 'lost', 6, 'lost'
);

-- Rebuild deals without the CHECK that fixed the stage to the six names. SQLite cannot drop a CHECK in
-- place. Every row, id and uuid is kept. The runner switches foreign keys off for this file (first
-- line) and runs foreign_key_check before it commits.
-- Two triggers of activity_target read deals, SQLite refuses the rename while they exist. They are
-- dropped here and created again below, unchanged from migration 008.
DROP TRIGGER trg_activity_target_bi;
DROP TRIGGER trg_activity_target_bu;

CREATE TEMP TABLE _seq AS SELECT name, seq FROM sqlite_sequence WHERE name = 'deals';

CREATE TABLE deals_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    value REAL,
    stage TEXT NOT NULL DEFAULT 'lead',
    notes TEXT,
    closed_at TEXT,
    archived INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO deals_new (id, uuid, title, value, stage, notes, closed_at, archived, created_at, updated_at)
SELECT id, uuid, title, value, stage, notes, closed_at, archived, created_at, updated_at FROM deals;
DROP TABLE deals;
ALTER TABLE deals_new RENAME TO deals;
CREATE INDEX idx_deals_stage ON deals(stage) WHERE archived = 0;

CREATE TRIGGER deals_ai AFTER INSERT ON deals BEGIN
    INSERT INTO deals_fts(rowid, title, notes)
    VALUES (new.id, new.title, new.notes);
END;
CREATE TRIGGER deals_ad AFTER DELETE ON deals BEGIN
    INSERT INTO deals_fts(deals_fts, rowid, title, notes)
    VALUES ('delete', old.id, old.title, old.notes);
END;
CREATE TRIGGER deals_au AFTER UPDATE ON deals BEGIN
    INSERT INTO deals_fts(deals_fts, rowid, title, notes)
    VALUES ('delete', old.id, old.title, old.notes);
    INSERT INTO deals_fts(rowid, title, notes)
    VALUES (new.id, new.title, new.notes);
END;
CREATE TRIGGER trg_deals_links_ad AFTER DELETE ON deals BEGIN
    DELETE FROM activity_target WHERE target_kind = 'deal' AND target_id = OLD.id;
END;

CREATE TRIGGER trg_activity_target_bi BEFORE INSERT ON activity_target BEGIN
    SELECT RAISE(ABORT, 'activity_target: source does not exist')
    WHERE NOT EXISTS (SELECT 1 FROM tasks WHERE NEW.source_kind = 'task' AND id = NEW.source_id)
      AND NOT EXISTS (SELECT 1 FROM interactions WHERE NEW.source_kind = 'interaction' AND id = NEW.source_id);
    SELECT RAISE(ABORT, 'activity_target: target does not exist')
    WHERE NOT EXISTS (SELECT 1 FROM people WHERE NEW.target_kind = 'person' AND id = NEW.target_id)
      AND NOT EXISTS (SELECT 1 FROM organizations WHERE NEW.target_kind = 'organization' AND id = NEW.target_id)
      AND NOT EXISTS (SELECT 1 FROM deals WHERE NEW.target_kind = 'deal' AND id = NEW.target_id);
END;
CREATE TRIGGER trg_activity_target_bu BEFORE UPDATE OF source_kind, source_id, target_kind, target_id ON activity_target BEGIN
    SELECT RAISE(ABORT, 'activity_target: source does not exist')
    WHERE NOT EXISTS (SELECT 1 FROM tasks WHERE NEW.source_kind = 'task' AND id = NEW.source_id)
      AND NOT EXISTS (SELECT 1 FROM interactions WHERE NEW.source_kind = 'interaction' AND id = NEW.source_id);
    SELECT RAISE(ABORT, 'activity_target: target does not exist')
    WHERE NOT EXISTS (SELECT 1 FROM people WHERE NEW.target_kind = 'person' AND id = NEW.target_id)
      AND NOT EXISTS (SELECT 1 FROM organizations WHERE NEW.target_kind = 'organization' AND id = NEW.target_id)
      AND NOT EXISTS (SELECT 1 FROM deals WHERE NEW.target_kind = 'deal' AND id = NEW.target_id);
END;

-- Keep the id counter where it was.
UPDATE sqlite_sequence SET seq = MAX(seq, COALESCE((SELECT seq FROM _seq), 0)) WHERE name = 'deals';
INSERT INTO sqlite_sequence (name, seq)
SELECT 'deals', seq FROM _seq WHERE NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'deals');
DROP TABLE _seq;

-- down

DELETE FROM vocabulary WHERE kind = 'deal_stage';
