-- up

-- The former `membership` table is the Person–Organization link. The old name was a
-- storage detail that leaked into the product model and public interfaces.
ALTER TABLE membership RENAME TO person_organization_link;

-- down

ALTER TABLE person_organization_link RENAME TO membership;
