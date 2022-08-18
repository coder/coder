-- Valid licenses don't fit into old format, so delete all data
DELETE FROM licenses;
ALTER TABLE licenses DROP COLUMN jwt;
ALTER TABLE licenses RENAME COLUMN uploaded_at to created_at;
ALTER TABLE licenses ADD COLUMN license jsonb NOT NULL;
ALTER TABLE licenses DROP COLUMN exp;

