-- We need to assign uuids to any existing licenses that don't have them.
UPDATE licenses SET uuid = gen_random_uuid() WHERE uuid IS NULL;
-- Assert no licenses have null uuids.
ALTER TABLE ONLY licenses ALTER COLUMN uuid SET NOT NULL;
