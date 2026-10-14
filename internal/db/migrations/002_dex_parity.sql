-- up

-- Multiple emails per person (Dex: contact_emails). people.email stays as the primary.
CREATE TABLE IF NOT EXISTS person_emails (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    person_id INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    label TEXT,
    is_primary INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(person_id, email)
);
CREATE INDEX IF NOT EXISTS idx_person_emails_email ON person_emails(email);

-- Multiple phones per person (Dex: contact_phone_numbers). people.phone stays as the primary.
CREATE TABLE IF NOT EXISTS person_phones (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    person_id INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    phone_number TEXT NOT NULL,
    label TEXT,
    country_code TEXT,
    is_primary INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(person_id, phone_number)
);
CREATE INDEX IF NOT EXISTS idx_person_phones_number ON person_phones(phone_number);

-- Social links and websites (Dex: linkedin, facebook, instagram, twitter, telegram, whatsapp, tiktok, youtube)
CREATE TABLE IF NOT EXISTS person_handles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    person_id INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK(kind IN ('linkedin', 'twitter', 'facebook', 'instagram', 'telegram', 'whatsapp', 'tiktok', 'youtube', 'website', 'other')),
    value TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(person_id, kind)
);
CREATE INDEX IF NOT EXISTS idx_person_handles_person ON person_handles(person_id);

-- Backfill the new tables from the single-value columns
INSERT OR IGNORE INTO person_emails (person_id, email, is_primary)
SELECT id, email, 1 FROM people WHERE email IS NOT NULL AND TRIM(email) != '';

INSERT OR IGNORE INTO person_phones (person_id, phone_number, is_primary)
SELECT id, phone, 1 FROM people WHERE phone IS NOT NULL AND TRIM(phone) != '';

-- Birthday (year optional), keep-in-touch, favorite, priority tier
ALTER TABLE people ADD COLUMN birthday_month INTEGER CHECK(birthday_month BETWEEN 1 AND 12);
ALTER TABLE people ADD COLUMN birthday_day INTEGER CHECK(birthday_day BETWEEN 1 AND 31);
ALTER TABLE people ADD COLUMN birthday_year INTEGER;
ALTER TABLE people ADD COLUMN keep_in_touch TEXT CHECK(keep_in_touch IN ('7 days', '14 days', '1 mon', '42 days', '3 mons', '6 mons', '1 year', 'never'));
ALTER TABLE people ADD COLUMN keep_in_touch_snoozed_until TEXT;
ALTER TABLE people ADD COLUMN last_contacted_at TEXT;
ALTER TABLE people ADD COLUMN is_favorite INTEGER NOT NULL DEFAULT 0;
ALTER TABLE people ADD COLUMN priority_tier INTEGER;

-- Recurring reminders linked to several people (Dex: reminders, reminders_contacts)
ALTER TABLE tasks ADD COLUMN recurrence TEXT;
ALTER TABLE tasks ADD COLUMN snoozed_until TEXT;

CREATE TABLE IF NOT EXISTS task_people (
    task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    person_id INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    PRIMARY KEY (task_id, person_id)
);
CREATE INDEX IF NOT EXISTS idx_task_people_person ON task_people(person_id);

INSERT OR IGNORE INTO task_people (task_id, person_id)
SELECT id, person_id FROM tasks WHERE person_id IS NOT NULL;

-- Keep-in-touch status. last_seen is the later of the manual value and the latest
-- non-note interaction. due_at is last_seen (or created_at) plus the cadence.
CREATE VIEW IF NOT EXISTS keep_in_touch_status AS
SELECT
    p.id AS person_id,
    p.first_name,
    p.last_name,
    p.keep_in_touch,
    p.keep_in_touch_snoozed_until,
    ls.last_seen,
    CASE p.keep_in_touch
        WHEN '7 days'  THEN datetime(COALESCE(ls.last_seen, p.created_at), '+7 days')
        WHEN '14 days' THEN datetime(COALESCE(ls.last_seen, p.created_at), '+14 days')
        WHEN '1 mon'   THEN datetime(COALESCE(ls.last_seen, p.created_at), '+1 month')
        WHEN '42 days' THEN datetime(COALESCE(ls.last_seen, p.created_at), '+42 days')
        WHEN '3 mons'  THEN datetime(COALESCE(ls.last_seen, p.created_at), '+3 months')
        WHEN '6 mons'  THEN datetime(COALESCE(ls.last_seen, p.created_at), '+6 months')
        WHEN '1 year'  THEN datetime(COALESCE(ls.last_seen, p.created_at), '+1 year')
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
        LEFT JOIN interaction_people ip ON ip.person_id = p2.id
        LEFT JOIN interactions i ON i.id = ip.interaction_id AND i.type != 'note' AND i.archived = 0
        GROUP BY p2.id
    )
) ls ON ls.person_id = p.id
WHERE p.archived = 0
  AND p.keep_in_touch IS NOT NULL
  AND p.keep_in_touch != 'never';

-- down

DROP VIEW IF EXISTS keep_in_touch_status;
DROP TABLE IF EXISTS task_people;
ALTER TABLE tasks DROP COLUMN snoozed_until;
ALTER TABLE tasks DROP COLUMN recurrence;
ALTER TABLE people DROP COLUMN priority_tier;
ALTER TABLE people DROP COLUMN is_favorite;
ALTER TABLE people DROP COLUMN last_contacted_at;
ALTER TABLE people DROP COLUMN keep_in_touch_snoozed_until;
ALTER TABLE people DROP COLUMN keep_in_touch;
ALTER TABLE people DROP COLUMN birthday_year;
ALTER TABLE people DROP COLUMN birthday_day;
ALTER TABLE people DROP COLUMN birthday_month;
DROP TABLE IF EXISTS person_handles;
DROP TABLE IF EXISTS person_phones;
DROP TABLE IF EXISTS person_emails;
