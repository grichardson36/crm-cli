-- up

-- Phone numbers in E.164 (owner decision 2026-10-27, patterns/phone-numbers-are-e164). phone_number
-- stays exactly as typed. e164 is the normalized form (+14155550100) and is NULL for text that cannot
-- be parsed into a possible number. `crm phone normalize --apply` fills it for existing rows.
ALTER TABLE person_phones ADD COLUMN e164 TEXT;
CREATE INDEX IF NOT EXISTS idx_person_phones_e164 ON person_phones(e164);

-- CardDAV: the last raw vCard text the phone and the server agreed on, so a later GET changes only the
-- properties whose field moved and keeps every other line byte for byte (decision Q14 to Q16).
-- raw_source is 'device' (the phone's own PUT) or 'server' (rendered by the CRM).
ALTER TABLE dav_state ADD COLUMN raw_vcard TEXT;
ALTER TABLE dav_state ADD COLUMN raw_source TEXT;

-- Sync issues: a phone vCard that could not be read is skipped and flagged here, never read as an
-- emptied contact. person_id is NULL when the contact is new.
CREATE TABLE IF NOT EXISTS dav_issues (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    dav_id TEXT NOT NULL,
    person_id INTEGER REFERENCES people(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    message TEXT NOT NULL,
    raw_vcard TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    resolved_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_dav_issues_open ON dav_issues(resolved_at, id);

-- down

DROP INDEX IF EXISTS idx_dav_issues_open;
DROP TABLE IF EXISTS dav_issues;
ALTER TABLE dav_state DROP COLUMN raw_source;
ALTER TABLE dav_state DROP COLUMN raw_vcard;
DROP INDEX IF EXISTS idx_person_phones_e164;
ALTER TABLE person_phones DROP COLUMN e164;
