-- No valid licenses should exist, but to be sure, drop all rows
DELETE FROM licenses;
ALTER TABLE licenses DROP COLUMN license;
ALTER TABLE licenses RENAME COLUMN created_at to uploaded_at;
ALTER TABLE licenses ADD COLUMN jwt text NOT NULL;
ALTER TABLE licenses ADD COLUMN exp timestamp with time zone NOT NULL;

