-- up

-- Booking links, Tier 0 (no server). A link says which times may be offered. Nothing here is
-- published by itself: `crm book offer` and `crm book export` render slots from it.
CREATE TABLE IF NOT EXISTS booking_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    title TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL DEFAULT 'one_on_one' CHECK(kind IN ('one_on_one')),
    host_email TEXT NOT NULL DEFAULT '',
    durations_json TEXT NOT NULL DEFAULT '[30]',
    schedule_name TEXT NOT NULL DEFAULT '',
    horizon_days INTEGER NOT NULL DEFAULT 14 CHECK(horizon_days > 0),
    start_after TEXT,
    min_notice_min INTEGER NOT NULL DEFAULT 240 CHECK(min_notice_min >= 0),
    slot_step_min INTEGER NOT NULL DEFAULT 30 CHECK(slot_step_min > 0),
    buffer_before_min INTEGER NOT NULL DEFAULT 0 CHECK(buffer_before_min >= 0),
    buffer_after_min INTEGER NOT NULL DEFAULT 0 CHECK(buffer_after_min >= 0),
    max_per_day INTEGER CHECK(max_per_day IS NULL OR max_per_day > 0),
    max_per_week INTEGER CHECK(max_per_week IS NULL OR max_per_week > 0),
    max_per_month INTEGER CHECK(max_per_month IS NULL OR max_per_month > 0),
    busy_tasks INTEGER NOT NULL DEFAULT 0,
    location_kind TEXT NOT NULL DEFAULT '' CHECK(location_kind IN ('', 'phone', 'in_person', 'url')),
    location TEXT NOT NULL DEFAULT '',
    questions_json TEXT NOT NULL DEFAULT '[]',
    reminders_json TEXT NOT NULL DEFAULT '[]',
    followup_after_min INTEGER CHECK(followup_after_min IS NULL OR followup_after_min >= 0),
    max_uses INTEGER CHECK(max_uses IS NULL OR max_uses > 0),
    expires_at TEXT,
    active INTEGER NOT NULL DEFAULT 1,
    archived INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- One row per booked meeting. event_id is the calendar event created on confirm. The row and the
-- event share the time, so a cancelled booking archives both.
CREATE TABLE IF NOT EXISTS bookings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    link_id INTEGER NOT NULL REFERENCES booking_links(id),
    event_id INTEGER REFERENCES events(id),
    person_id INTEGER REFERENCES people(id),
    invitee_name TEXT NOT NULL DEFAULT '',
    invitee_email TEXT NOT NULL,
    invitee_tz TEXT NOT NULL DEFAULT '',
    answers_json TEXT NOT NULL DEFAULT '{}',
    start_utc TEXT NOT NULL,
    end_utc TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'confirmed' CHECK(status IN ('requested', 'confirmed', 'cancelled', 'rescheduled')),
    reminded_json TEXT NOT NULL DEFAULT '[]',
    followup_sent INTEGER NOT NULL DEFAULT 0,
    archived INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_bookings_link_start ON bookings(link_id, start_utc) WHERE archived = 0;
CREATE INDEX IF NOT EXISTS idx_bookings_start ON bookings(start_utc) WHERE archived = 0;

-- down

DROP TABLE IF EXISTS bookings;
DROP TABLE IF EXISTS booking_links;
