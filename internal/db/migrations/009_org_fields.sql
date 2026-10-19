-- up

-- Native organization fields (M15), as HubSpot and Salesforce keep them on a company record.
-- All optional text. size is free text, the suggested buckets are in the program (1-10, 11-50,
-- 51-200, 201-500, 501-1000, 1001-5000, 5000+). location is three columns so a list can filter by
-- country or city. logo_path is a path only, nothing draws it yet. The version a database gets is
-- the position of the file, so never renumber.
ALTER TABLE organizations ADD COLUMN website TEXT;
ALTER TABLE organizations ADD COLUMN size TEXT;
ALTER TABLE organizations ADD COLUMN city TEXT;
ALTER TABLE organizations ADD COLUMN region TEXT;
ALTER TABLE organizations ADD COLUMN country TEXT;
ALTER TABLE organizations ADD COLUMN linkedin_url TEXT;
ALTER TABLE organizations ADD COLUMN description TEXT;
ALTER TABLE organizations ADD COLUMN logo_path TEXT;

-- down

ALTER TABLE organizations DROP COLUMN logo_path;
ALTER TABLE organizations DROP COLUMN description;
ALTER TABLE organizations DROP COLUMN linkedin_url;
ALTER TABLE organizations DROP COLUMN country;
ALTER TABLE organizations DROP COLUMN region;
ALTER TABLE organizations DROP COLUMN city;
ALTER TABLE organizations DROP COLUMN size;
ALTER TABLE organizations DROP COLUMN website;
