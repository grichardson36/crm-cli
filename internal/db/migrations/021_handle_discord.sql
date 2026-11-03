-- up

-- Discord joins the social handle kinds (owner request 2026-11-03). SQLite cannot change a CHECK in
-- place, and person_handles may carry a uuid column and capture triggers when device sync is on, so
-- a table rebuild would lose them. The CHECK text is edited in the schema instead: the table, its
-- rows and its indexes are untouched. The runner resets the schema cache after each migration.
-- The version a database gets is the position of the file, so never renumber.
PRAGMA writable_schema = ON;
UPDATE sqlite_master
SET sql = replace(sql, '''youtube'', ''website''', '''youtube'', ''discord'', ''website''')
WHERE type = 'table' AND name = 'person_handles';
PRAGMA writable_schema = OFF;

-- down

PRAGMA writable_schema = ON;
UPDATE sqlite_master
SET sql = replace(sql, '''youtube'', ''discord'', ''website''', '''youtube'', ''website''')
WHERE type = 'table' AND name = 'person_handles';
PRAGMA writable_schema = OFF;
