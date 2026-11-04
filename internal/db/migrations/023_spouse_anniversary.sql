-- up

-- CRM-022: an anniversary on a spouse link. The date is stored on the link row, so both partners share
-- it (a symmetric kind is stored once). It is YYYY-MM-DD or --MM-DD when the year is unknown, like a
-- birthday. Only the built-in spouse kind takes one, the link code enforces that.
ALTER TABLE person_relation ADD COLUMN anniversary TEXT;

-- down

DROP TRIGGER IF EXISTS rpl_person_relation_ai;
DROP TRIGGER IF EXISTS rpl_person_relation_au;
DROP TRIGGER IF EXISTS rpl_person_relation_ad;
ALTER TABLE person_relation DROP COLUMN anniversary;
