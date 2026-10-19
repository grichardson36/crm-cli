-- foreign_keys: off
-- up

-- Relations that match the real world (patterns/schema-er.md, approved 2026-10-16).
--
-- New: membership, relation_kind, person_relation, org_relation, deal_person, deal_org,
-- activity_target, merge_alias. Every one has id, uuid, created_at, updated_at and deleted_at.
-- Removed after the data moves: people.org_id, deals.person_id, deals.org_id, tasks.person_id,
-- tasks.deal_id and the tables relationships, task_people, interaction_people.
-- SQLite cannot drop a column that has a foreign key, so people, deals and tasks are rebuilt with
-- every row, id and uuid kept. The runner switches foreign keys off for this file (first line) and
-- runs foreign_key_check before it commits. The version a database gets is the position of the
-- file, so never renumber.

DROP VIEW IF EXISTS keep_in_touch_status;

-- Vocabulary of person and organization link kinds ------------------------------------------
-- A row (a, b, kind) reads "a is <name> b". Seen from b, a is shown as name. Seen from a, b is
-- shown as inverse_name. A symmetric kind has name = inverse_name and is stored once, lower id first.
CREATE TABLE relation_kind (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    object_type TEXT NOT NULL CHECK(object_type IN ('person', 'organization')),
    name TEXT NOT NULL COLLATE NOCASE,
    inverse_name TEXT NOT NULL COLLATE NOCASE,
    symmetric INTEGER NOT NULL DEFAULT 0 CHECK(symmetric IN (0, 1)),
    builtin INTEGER NOT NULL DEFAULT 0 CHECK(builtin IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    deleted_at TEXT,
    CHECK(symmetric = 0 OR name = inverse_name)
);
CREATE UNIQUE INDEX idx_relation_kind_name ON relation_kind(object_type, name) WHERE deleted_at IS NULL;

-- Built-in kinds have fixed uuids so every device agrees on them.
INSERT INTO relation_kind (uuid, object_type, name, inverse_name, symmetric, builtin) VALUES
    ('a0000001-0000-4000-8000-000000000001', 'person', 'related', 'related', 1, 1),
    ('a0000001-0000-4000-8000-000000000002', 'person', 'spouse', 'spouse', 1, 1),
    ('a0000001-0000-4000-8000-000000000003', 'person', 'sibling', 'sibling', 1, 1),
    ('a0000001-0000-4000-8000-000000000004', 'person', 'friend', 'friend', 1, 1),
    ('a0000001-0000-4000-8000-000000000005', 'person', 'colleague', 'colleague', 1, 1),
    ('a0000001-0000-4000-8000-000000000006', 'person', 'parent', 'child', 0, 1),
    ('a0000001-0000-4000-8000-000000000007', 'person', 'manager', 'report', 0, 1),
    ('a0000001-0000-4000-8000-000000000008', 'person', 'mentor', 'mentee', 0, 1),
    ('a0000001-0000-4000-8000-000000000009', 'person', 'referred-by', 'referred', 0, 1),
    ('a0000002-0000-4000-8000-000000000001', 'organization', 'parent', 'child', 0, 1);

-- Person to organization, many to many, with role, dates and a primary flag ------------------
CREATE TABLE membership (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    person_id INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    org_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    role TEXT,
    started_on TEXT,
    ended_on TEXT,
    is_primary INTEGER NOT NULL DEFAULT 0 CHECK(is_primary IN (0, 1)),
    notes TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    deleted_at TEXT,
    CHECK(started_on IS NULL OR ended_on IS NULL OR started_on <= ended_on)
);
CREATE UNIQUE INDEX uq_membership_primary ON membership(person_id) WHERE is_primary = 1 AND deleted_at IS NULL;
CREATE INDEX idx_membership_person ON membership(person_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_membership_org ON membership(org_id) WHERE deleted_at IS NULL;

-- Person to person and organization to organization, typed ----------------------------------
CREATE TABLE person_relation (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    a_id INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    b_id INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    kind_id INTEGER NOT NULL REFERENCES relation_kind(id),
    notes TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    deleted_at TEXT,
    CHECK(a_id <> b_id)
);
CREATE UNIQUE INDEX uq_person_relation ON person_relation(a_id, b_id, kind_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_person_relation_b ON person_relation(b_id) WHERE deleted_at IS NULL;

CREATE TABLE org_relation (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    a_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    b_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    kind_id INTEGER NOT NULL REFERENCES relation_kind(id),
    notes TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    deleted_at TEXT,
    CHECK(a_id <> b_id)
);
CREATE UNIQUE INDEX uq_org_relation ON org_relation(a_id, b_id, kind_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_org_relation_b ON org_relation(b_id) WHERE deleted_at IS NULL;

-- Deals to people and organizations ---------------------------------------------------------
CREATE TABLE deal_person (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    deal_id INTEGER NOT NULL REFERENCES deals(id) ON DELETE CASCADE,
    person_id INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    role TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    deleted_at TEXT
);
CREATE UNIQUE INDEX uq_deal_person ON deal_person(deal_id, person_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_deal_person_person ON deal_person(person_id) WHERE deleted_at IS NULL;

CREATE TABLE deal_org (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    deal_id INTEGER NOT NULL REFERENCES deals(id) ON DELETE CASCADE,
    org_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    is_primary INTEGER NOT NULL DEFAULT 0 CHECK(is_primary IN (0, 1)),
    role TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    deleted_at TEXT
);
CREATE UNIQUE INDEX uq_deal_org ON deal_org(deal_id, org_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_deal_org_primary ON deal_org(deal_id) WHERE is_primary = 1 AND deleted_at IS NULL;
CREATE INDEX idx_deal_org_org ON deal_org(org_id) WHERE deleted_at IS NULL;

-- Tasks and interactions about people, organizations and deals ------------------------------
-- The source and target columns are polymorphic, so no foreign keys. Triggers below keep the
-- table free of orphans and crm doctor checks it.
CREATE TABLE activity_target (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    source_kind TEXT NOT NULL CHECK(source_kind IN ('task', 'interaction')),
    source_id INTEGER NOT NULL,
    target_kind TEXT NOT NULL CHECK(target_kind IN ('person', 'organization', 'deal')),
    target_id INTEGER NOT NULL,
    role TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    deleted_at TEXT
);
CREATE UNIQUE INDEX uq_activity_target ON activity_target(source_kind, source_id, target_kind, target_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_activity_target_target ON activity_target(target_kind, target_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_activity_target_source ON activity_target(source_kind, source_id);

-- Old ids and uuids of merged records keep resolving ----------------------------------------
CREATE TABLE merge_alias (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL CHECK(kind IN ('person', 'organization')),
    old_id INTEGER NOT NULL,
    old_uuid TEXT,
    survivor_id INTEGER NOT NULL,
    merged_at TEXT NOT NULL DEFAULT (datetime('now')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    deleted_at TEXT
);
CREATE UNIQUE INDEX uq_merge_alias ON merge_alias(kind, old_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_merge_alias_survivor ON merge_alias(kind, survivor_id) WHERE deleted_at IS NULL;

-- Move the data -----------------------------------------------------------------------------

INSERT INTO membership (uuid, person_id, org_id, is_primary)
SELECT lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)), 2) || '-' ||
             substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2) || '-' || hex(randomblob(6))),
       id, org_id, 1
FROM people WHERE org_id IS NOT NULL;

INSERT INTO deal_person (uuid, deal_id, person_id)
SELECT lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)), 2) || '-' ||
             substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2) || '-' || hex(randomblob(6))),
       id, person_id
FROM deals WHERE person_id IS NOT NULL;

INSERT INTO deal_org (uuid, deal_id, org_id, is_primary)
SELECT lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)), 2) || '-' ||
             substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2) || '-' || hex(randomblob(6))),
       id, org_id, 1
FROM deals WHERE org_id IS NOT NULL;

INSERT OR IGNORE INTO activity_target (uuid, source_kind, source_id, target_kind, target_id)
SELECT lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)), 2) || '-' ||
             substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2) || '-' || hex(randomblob(6))),
       'task', id, 'person', person_id
FROM tasks WHERE person_id IS NOT NULL;

INSERT OR IGNORE INTO activity_target (uuid, source_kind, source_id, target_kind, target_id)
SELECT lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)), 2) || '-' ||
             substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2) || '-' || hex(randomblob(6))),
       'task', task_id, 'person', person_id
FROM task_people;

INSERT OR IGNORE INTO activity_target (uuid, source_kind, source_id, target_kind, target_id)
SELECT lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)), 2) || '-' ||
             substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2) || '-' || hex(randomblob(6))),
       'task', id, 'deal', deal_id
FROM tasks WHERE deal_id IS NOT NULL;

INSERT OR IGNORE INTO activity_target (uuid, source_kind, source_id, target_kind, target_id)
SELECT lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)), 2) || '-' ||
             substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2) || '-' || hex(randomblob(6))),
       'interaction', interaction_id, 'person', person_id
FROM interaction_people;

-- Old person to person relationships. The old type is the new kind name, so the mapping is by name.
-- The old row (person_id, related_person_id, type) becomes (a, b, kind) in the same order.
INSERT OR IGNORE INTO person_relation (uuid, a_id, b_id, kind_id, notes, created_at)
SELECT lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)), 2) || '-' ||
             substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2) || '-' || hex(randomblob(6))),
       CASE WHEN k.symmetric = 1 AND r.related_person_id < r.person_id THEN r.related_person_id ELSE r.person_id END,
       CASE WHEN k.symmetric = 1 AND r.related_person_id < r.person_id THEN r.person_id ELSE r.related_person_id END,
       k.id, r.notes, r.created_at
FROM relationships r
JOIN relation_kind k ON k.object_type = 'person' AND k.name = r.type;

-- Dex "related contacts" custom field: a comma separated list of names (or Dex ids). An item is
-- resolved through the dex id in external_ids, else by an exact full name with exactly one match.
CREATE TEMP TABLE _rc_items AS
WITH RECURSIVE split(pid, rest, item) AS (
    SELECT entity_id, field_value || ',', '' FROM custom_fields
    WHERE entity_type = 'person' AND field_name = 'related_contacts' AND TRIM(field_value) != ''
    UNION ALL
    SELECT pid, substr(rest, instr(rest, ',') + 1), trim(substr(rest, 1, instr(rest, ',') - 1)) FROM split WHERE rest <> ''
)
SELECT pid, item,
       COALESCE(
         (SELECT e.local_id FROM external_ids e WHERE e.source = 'dex' AND e.entity = 'person' AND e.external_id = split.item),
         CASE WHEN (SELECT COUNT(*) FROM people q WHERE q.id <> split.pid
                    AND lower(trim(q.first_name || ' ' || COALESCE(q.last_name, ''))) = lower(split.item)) = 1
              THEN (SELECT q.id FROM people q WHERE q.id <> split.pid
                    AND lower(trim(q.first_name || ' ' || COALESCE(q.last_name, ''))) = lower(split.item)) END
       ) AS qid
FROM split WHERE item <> '';

INSERT OR IGNORE INTO person_relation (uuid, a_id, b_id, kind_id)
SELECT lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)), 2) || '-' ||
             substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2) || '-' || hex(randomblob(6))),
       MIN(pid, qid), MAX(pid, qid), (SELECT id FROM relation_kind WHERE object_type = 'person' AND name = 'related')
FROM _rc_items WHERE qid IS NOT NULL AND qid <> pid;

-- Report of what moved, readable with: SELECT value FROM meta WHERE key = 'migration_008'
INSERT OR REPLACE INTO meta (key, value) VALUES ('migration_008',
    'membership=' || (SELECT COUNT(*) FROM membership) ||
    ' deal_person=' || (SELECT COUNT(*) FROM deal_person) ||
    ' deal_org=' || (SELECT COUNT(*) FROM deal_org) ||
    ' activity_target=' || (SELECT COUNT(*) FROM activity_target) ||
    ' person_relation_from_relationships=' || (SELECT COUNT(*) FROM relationships) ||
    ' person_relation=' || (SELECT COUNT(*) FROM person_relation) ||
    ' related_contacts_items=' || (SELECT COUNT(*) FROM _rc_items) ||
    ' related_contacts_resolved=' || (SELECT COUNT(*) FROM _rc_items WHERE qid IS NOT NULL) ||
    ' related_contacts_unresolved=' || (SELECT COUNT(*) FROM _rc_items WHERE qid IS NULL));
DROP TABLE _rc_items;

-- Remember the id counters, the rebuilds below must not lower them.
CREATE TEMP TABLE _seq AS SELECT name, seq FROM sqlite_sequence WHERE name IN ('people', 'deals', 'tasks');

-- Rebuild people without org_id ---------------------------------------------------------------
CREATE TABLE people_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    first_name TEXT NOT NULL,
    last_name TEXT,
    email TEXT,
    phone TEXT,
    title TEXT,
    company TEXT,
    location TEXT,
    notes TEXT,
    summary TEXT,
    archived INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    birthday_month INTEGER CHECK(birthday_month BETWEEN 1 AND 12),
    birthday_day INTEGER CHECK(birthday_day BETWEEN 1 AND 31),
    birthday_year INTEGER,
    keep_in_touch TEXT CHECK(keep_in_touch IN ('7 days', '14 days', '1 mon', '42 days', '3 mons', '6 mons', '1 year', 'never')),
    keep_in_touch_snoozed_until TEXT,
    last_contacted_at TEXT,
    is_favorite INTEGER NOT NULL DEFAULT 0,
    priority_tier INTEGER,
    merged_into INTEGER REFERENCES people(id),
    keep_in_touch_set_at TEXT
);
INSERT INTO people_new (id, uuid, first_name, last_name, email, phone, title, company, location, notes, summary,
    archived, created_at, updated_at, birthday_month, birthday_day, birthday_year, keep_in_touch,
    keep_in_touch_snoozed_until, last_contacted_at, is_favorite, priority_tier, merged_into, keep_in_touch_set_at)
SELECT id, uuid, first_name, last_name, email, phone, title, company, location, notes, summary,
    archived, created_at, updated_at, birthday_month, birthday_day, birthday_year, keep_in_touch,
    keep_in_touch_snoozed_until, last_contacted_at, is_favorite, priority_tier, merged_into, keep_in_touch_set_at
FROM people;
DROP TABLE people;
ALTER TABLE people_new RENAME TO people;
CREATE INDEX idx_people_email ON people(email) WHERE archived = 0;
CREATE INDEX idx_people_merged_into ON people(merged_into) WHERE merged_into IS NOT NULL;

CREATE TRIGGER people_ai AFTER INSERT ON people BEGIN
    INSERT INTO people_fts(rowid, first_name, last_name, email, company, notes)
    VALUES (new.id, new.first_name, new.last_name, new.email, new.company, new.notes);
END;
CREATE TRIGGER people_ad AFTER DELETE ON people BEGIN
    INSERT INTO people_fts(people_fts, rowid, first_name, last_name, email, company, notes)
    VALUES ('delete', old.id, old.first_name, old.last_name, old.email, old.company, old.notes);
END;
CREATE TRIGGER people_au AFTER UPDATE ON people BEGIN
    INSERT INTO people_fts(people_fts, rowid, first_name, last_name, email, company, notes)
    VALUES ('delete', old.id, old.first_name, old.last_name, old.email, old.company, old.notes);
    INSERT INTO people_fts(rowid, first_name, last_name, email, company, notes)
    VALUES (new.id, new.first_name, new.last_name, new.email, new.company, new.notes);
END;

-- Rebuild deals without person_id and org_id --------------------------------------------------
CREATE TABLE deals_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    value REAL,
    stage TEXT NOT NULL DEFAULT 'lead' CHECK(stage IN ('lead', 'prospect', 'proposal', 'negotiation', 'won', 'lost')),
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

-- Rebuild tasks without person_id and deal_id -------------------------------------------------
CREATE TABLE tasks_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    description TEXT,
    due_at TEXT,
    priority TEXT NOT NULL DEFAULT 'medium' CHECK(priority IN ('low', 'medium', 'high')),
    completed INTEGER NOT NULL DEFAULT 0,
    completed_at TEXT,
    archived INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    recurrence TEXT,
    snoozed_until TEXT
);
INSERT INTO tasks_new (id, uuid, title, description, due_at, priority, completed, completed_at, archived,
    created_at, updated_at, recurrence, snoozed_until)
SELECT id, uuid, title, description, due_at, priority, completed, completed_at, archived,
    created_at, updated_at, recurrence, snoozed_until FROM tasks;
DROP TABLE tasks;
ALTER TABLE tasks_new RENAME TO tasks;
CREATE INDEX idx_tasks_due ON tasks(due_at) WHERE archived = 0 AND completed = 0;

-- Keep the id counters where they were.
UPDATE sqlite_sequence SET seq = MAX(seq, COALESCE((SELECT seq FROM _seq WHERE _seq.name = sqlite_sequence.name), 0))
WHERE name IN ('people', 'deals', 'tasks');
DROP TABLE _seq;

-- Drop the old link tables --------------------------------------------------------------------
DROP TABLE task_people;
DROP TABLE interaction_people;
DROP TABLE relationships;

-- Orphan protection for activity_target and merge_alias (no foreign keys on polymorphic columns)
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
CREATE TRIGGER trg_people_links_ad AFTER DELETE ON people BEGIN
    DELETE FROM activity_target WHERE target_kind = 'person' AND target_id = OLD.id;
    DELETE FROM merge_alias WHERE kind = 'person' AND survivor_id = OLD.id;
END;
CREATE TRIGGER trg_orgs_links_ad AFTER DELETE ON organizations BEGIN
    DELETE FROM activity_target WHERE target_kind = 'organization' AND target_id = OLD.id;
    DELETE FROM merge_alias WHERE kind = 'organization' AND survivor_id = OLD.id;
END;
CREATE TRIGGER trg_deals_links_ad AFTER DELETE ON deals BEGIN
    DELETE FROM activity_target WHERE target_kind = 'deal' AND target_id = OLD.id;
END;
CREATE TRIGGER trg_tasks_links_ad AFTER DELETE ON tasks BEGIN
    DELETE FROM activity_target WHERE source_kind = 'task' AND source_id = OLD.id;
END;
CREATE TRIGGER trg_interactions_links_ad AFTER DELETE ON interactions BEGIN
    DELETE FROM activity_target WHERE source_kind = 'interaction' AND source_id = OLD.id;
END;

-- A link kind must belong to the object type of the table it is used in.
CREATE TRIGGER trg_person_relation_kind_bi BEFORE INSERT ON person_relation BEGIN
    SELECT RAISE(ABORT, 'person_relation: kind is not a person kind')
    WHERE NOT EXISTS (SELECT 1 FROM relation_kind WHERE id = NEW.kind_id AND object_type = 'person');
END;
CREATE TRIGGER trg_org_relation_kind_bi BEFORE INSERT ON org_relation BEGIN
    SELECT RAISE(ABORT, 'org_relation: kind is not an organization kind')
    WHERE NOT EXISTS (SELECT 1 FROM relation_kind WHERE id = NEW.kind_id AND object_type = 'organization');
END;

-- Keep-in-touch status, now reading interactions through activity_target. last_seen is the later
-- of the manual value and the latest interaction whose type counts as contact.
CREATE VIEW keep_in_touch_status AS
SELECT
    p.id AS person_id,
    p.first_name,
    p.last_name,
    p.keep_in_touch,
    p.keep_in_touch_snoozed_until,
    ls.last_seen,
    CASE p.keep_in_touch
        WHEN '7 days'  THEN datetime(COALESCE(ls.last_seen, p.keep_in_touch_set_at, p.created_at), '+7 days')
        WHEN '14 days' THEN datetime(COALESCE(ls.last_seen, p.keep_in_touch_set_at, p.created_at), '+14 days')
        WHEN '1 mon'   THEN datetime(COALESCE(ls.last_seen, p.keep_in_touch_set_at, p.created_at), '+1 month')
        WHEN '42 days' THEN datetime(COALESCE(ls.last_seen, p.keep_in_touch_set_at, p.created_at), '+42 days')
        WHEN '3 mons'  THEN datetime(COALESCE(ls.last_seen, p.keep_in_touch_set_at, p.created_at), '+3 months')
        WHEN '6 mons'  THEN datetime(COALESCE(ls.last_seen, p.keep_in_touch_set_at, p.created_at), '+6 months')
        WHEN '1 year'  THEN datetime(COALESCE(ls.last_seen, p.keep_in_touch_set_at, p.created_at), '+1 year')
    END AS due_at
FROM people p
LEFT JOIN (
    SELECT person_id,
           CASE
               WHEN manual IS NULL THEN derived
               WHEN derived IS NULL THEN manual
               ELSE MAX(manual, derived)
           END AS last_seen
    FROM (
        SELECT p2.id AS person_id,
               p2.last_contacted_at AS manual,
               MAX(i.occurred_at) AS derived
        FROM people p2
        LEFT JOIN activity_target at ON at.source_kind = 'interaction' AND at.target_kind = 'person'
             AND at.target_id = p2.id AND at.deleted_at IS NULL
        LEFT JOIN interactions i ON i.id = at.source_id AND i.archived = 0
             AND COALESCE((SELECT nt.counts_as_contact FROM note_types nt WHERE nt.name = i.type), 1) = 1
        GROUP BY p2.id
    )
) ls ON ls.person_id = p.id
WHERE p.archived = 0
  AND p.keep_in_touch IS NOT NULL
  AND p.keep_in_touch != 'never';

-- down

-- Not reversible in place. Restore the backup taken before the upgrade.
