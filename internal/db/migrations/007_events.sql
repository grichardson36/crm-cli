-- up

-- Events (J01) --------------------------------------------------------------------------
-- A timed event stores UTC instants (start_utc, end_utc) plus the IANA zone of its author, because
-- recurrence and display need the author's wall clock. An all-day event stores dates (start_date,
-- end_date exclusive) and no zone semantics: a birthday is on the same date everywhere.
-- A repeating event stores one rule (rrule, RFC 5545 body) and is expanded when read.
-- An override row (recurrence_parent_id + recurrence_id) replaces one instance of a series.
CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    notes TEXT,
    location TEXT,
    all_day INTEGER NOT NULL DEFAULT 0,
    start_utc TEXT,
    end_utc TEXT,
    start_date TEXT,
    end_date TEXT,
    tz TEXT,
    rrule TEXT,
    recurrence_parent_id INTEGER REFERENCES events(id) ON DELETE CASCADE,
    recurrence_id TEXT,
    status TEXT NOT NULL DEFAULT 'confirmed' CHECK(status IN ('confirmed', 'tentative', 'cancelled')),
    busy INTEGER NOT NULL DEFAULT 1,
    source TEXT NOT NULL DEFAULT 'local',
    floating INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    archived INTEGER NOT NULL DEFAULT 0,
    archived_at TEXT,
    CHECK ((all_day = 0 AND start_utc IS NOT NULL AND end_utc IS NOT NULL) OR (all_day = 1 AND start_date IS NOT NULL AND end_date IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_events_start_utc ON events(start_utc) WHERE archived = 0;
CREATE INDEX IF NOT EXISTS idx_events_start_date ON events(start_date) WHERE archived = 0;
CREATE INDEX IF NOT EXISTS idx_events_parent ON events(recurrence_parent_id);
CREATE INDEX IF NOT EXISTS idx_events_rrule ON events(rrule) WHERE rrule IS NOT NULL AND archived = 0;

-- Skipped instances of a series, identified by their original start (an instant, or a date for
-- all-day events).
CREATE TABLE IF NOT EXISTS event_exdates (
    event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    exdate TEXT NOT NULL,
    PRIMARY KEY (event_id, exdate)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS event_people (
    event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    person_id INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'attendee' CHECK(role IN ('organizer', 'attendee')),
    rsvp TEXT NOT NULL DEFAULT 'needs-action' CHECK(rsvp IN ('needs-action', 'accepted', 'declined', 'tentative')),
    PRIMARY KEY (event_id, person_id)
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_event_people_person ON event_people(person_id);

CREATE TABLE IF NOT EXISTS event_reminders (
    event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    minutes_before INTEGER NOT NULL CHECK(minutes_before >= 0),
    method TEXT NOT NULL DEFAULT 'display' CHECK(method IN ('display', 'email')),
    PRIMARY KEY (event_id, minutes_before, method)
) WITHOUT ROWID;

-- Full text search over events.
CREATE VIRTUAL TABLE IF NOT EXISTS events_fts USING fts5(
    title, notes, location,
    content='events', content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS events_ai AFTER INSERT ON events BEGIN
    INSERT INTO events_fts(rowid, title, notes, location)
    VALUES (new.id, new.title, new.notes, new.location);
END;

CREATE TRIGGER IF NOT EXISTS events_ad AFTER DELETE ON events BEGIN
    INSERT INTO events_fts(events_fts, rowid, title, notes, location)
    VALUES ('delete', old.id, old.title, old.notes, old.location);
END;

CREATE TRIGGER IF NOT EXISTS events_au AFTER UPDATE ON events BEGIN
    INSERT INTO events_fts(events_fts, rowid, title, notes, location)
    VALUES ('delete', old.id, old.title, old.notes, old.location);
    INSERT INTO events_fts(rowid, title, notes, location)
    VALUES (new.id, new.title, new.notes, new.location);
END;

-- Tags can be applied to events. The taggings CHECK lists the entity types, so the table is rebuilt.
CREATE TABLE taggings_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL CHECK(entity_type IN ('person', 'organization', 'deal', 'interaction', 'event')),
    entity_id INTEGER NOT NULL,
    UNIQUE(tag_id, entity_type, entity_id)
);
INSERT INTO taggings_new (id, tag_id, entity_type, entity_id) SELECT id, tag_id, entity_type, entity_id FROM taggings;
DROP TABLE taggings;
ALTER TABLE taggings_new RENAME TO taggings;
CREATE INDEX IF NOT EXISTS idx_taggings_entity ON taggings(entity_type, entity_id);

-- down

DELETE FROM taggings WHERE entity_type = 'event';
CREATE TABLE taggings_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL CHECK(entity_type IN ('person', 'organization', 'deal', 'interaction')),
    entity_id INTEGER NOT NULL,
    UNIQUE(tag_id, entity_type, entity_id)
);
INSERT INTO taggings_old (id, tag_id, entity_type, entity_id) SELECT id, tag_id, entity_type, entity_id FROM taggings;
DROP TABLE taggings;
ALTER TABLE taggings_old RENAME TO taggings;
CREATE INDEX IF NOT EXISTS idx_taggings_entity ON taggings(entity_type, entity_id);

DROP TRIGGER IF EXISTS events_au;
DROP TRIGGER IF EXISTS events_ad;
DROP TRIGGER IF EXISTS events_ai;
DROP TABLE IF EXISTS events_fts;
DROP TABLE IF EXISTS event_reminders;
DROP TABLE IF EXISTS event_people;
DROP TABLE IF EXISTS event_exdates;
DROP TABLE IF EXISTS events;
