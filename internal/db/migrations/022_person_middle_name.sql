-- up

-- CRM-019: a middle name for people. It is a detail field, found by search, and it travels as the
-- additional names component of the vCard N property. The display name stays "First Last".
-- The file is named 022 because the file named 017 does not exist, the version is the position of the file.
ALTER TABLE people ADD COLUMN middle_name TEXT;

-- The people search index lists its columns, so it is rebuilt with the new one. The triggers are the
-- same as before plus the column.
DROP TRIGGER IF EXISTS people_ai;
DROP TRIGGER IF EXISTS people_ad;
DROP TRIGGER IF EXISTS people_au;
DROP TABLE IF EXISTS people_fts;
CREATE VIRTUAL TABLE people_fts USING fts5(
    first_name, last_name, middle_name, email, company, notes,
    content='people', content_rowid='id'
);
CREATE TRIGGER people_ai AFTER INSERT ON people BEGIN
    INSERT INTO people_fts(rowid, first_name, last_name, middle_name, email, company, notes)
    VALUES (new.id, new.first_name, new.last_name, new.middle_name, new.email, new.company, new.notes);
END;
CREATE TRIGGER people_ad AFTER DELETE ON people BEGIN
    INSERT INTO people_fts(people_fts, rowid, first_name, last_name, middle_name, email, company, notes)
    VALUES ('delete', old.id, old.first_name, old.last_name, old.middle_name, old.email, old.company, old.notes);
END;
CREATE TRIGGER people_au AFTER UPDATE ON people BEGIN
    INSERT INTO people_fts(people_fts, rowid, first_name, last_name, middle_name, email, company, notes)
    VALUES ('delete', old.id, old.first_name, old.last_name, old.middle_name, old.email, old.company, old.notes);
    INSERT INTO people_fts(rowid, first_name, last_name, middle_name, email, company, notes)
    VALUES (new.id, new.first_name, new.last_name, new.middle_name, new.email, new.company, new.notes);
END;
INSERT INTO people_fts(people_fts) VALUES ('rebuild');

-- Until now the phone's additional name was kept aside in dav_state.extra_vcard (the third part of the
-- N line) and sent back unchanged. It becomes the field, and the merge base of that card learns it, so
-- the first edit on either side after the upgrade is not a conflict and the phone does not lose the name.
-- Cards whose N line holds a backslash pair are left alone (a stored escape cannot be told apart in SQL),
-- they still carry the name in their stored card and a later phone upload sets the field.
CREATE TEMP TABLE _mn_line AS
SELECT person_id,
       substr(rest, 1, instr(rest, char(13)) - 1) AS line
FROM (SELECT person_id, substr(extra_vcard, instr(extra_vcard, char(10) || 'N:') + 3) AS rest
      FROM dav_state
      WHERE extra_vcard LIKE '%' || char(10) || 'N:%')
WHERE instr(rest, char(13)) > 0;

CREATE TEMP TABLE _mn AS
SELECT person_id,
       replace(substr(r2, 1, instr(r2 || ';', ';') - 1), '\,', ',') AS mid
FROM (SELECT person_id, substr(r1, instr(r1, ';') + 1) AS r2
      FROM (SELECT person_id, substr(line, instr(line, ';') + 1) AS r1
            FROM _mn_line
            WHERE instr(line, '\\') = 0
              AND length(line) - length(replace(line, ';', '')) >= 2))
WHERE trim(replace(substr(r2, 1, instr(r2 || ';', ';') - 1), '\,', ',')) <> '';

UPDATE people SET middle_name = (SELECT trim(mid) FROM _mn WHERE _mn.person_id = people.id)
WHERE middle_name IS NULL AND id IN (SELECT person_id FROM _mn);

UPDATE dav_state SET base_json = json_set(base_json, '$.middle', (SELECT trim(mid) FROM _mn WHERE _mn.person_id = dav_state.person_id))
WHERE person_id IN (SELECT person_id FROM _mn) AND base_json IS NOT NULL AND json_valid(base_json);

DROP TABLE _mn;
DROP TABLE _mn_line;

-- down

DROP TRIGGER IF EXISTS rpl_people_ai;
DROP TRIGGER IF EXISTS rpl_people_au;
DROP TRIGGER IF EXISTS rpl_people_ad;
DROP TRIGGER IF EXISTS people_ai;
DROP TRIGGER IF EXISTS people_ad;
DROP TRIGGER IF EXISTS people_au;
DROP TABLE IF EXISTS people_fts;
CREATE VIRTUAL TABLE people_fts USING fts5(
    first_name, last_name, email, company, notes,
    content='people', content_rowid='id'
);
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
ALTER TABLE people DROP COLUMN middle_name;
INSERT INTO people_fts(people_fts) VALUES ('rebuild');
