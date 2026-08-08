-- No valid licenses should exist, but to be sure, drop all rows
DELETE FROM licenses;
ALTER TABLE licenses DROP COLUMN license;
ALTER TABLE licenses RENAME COLUMN created_at to uploaded_at;
ALTER TABLE licenses ADD COLUMN jwt text NOT NULL;
-- prevent adding the same license more than once
ALTER TABLE licenses ADD CONSTRAINT licenses_jwt_key UNIQUE (jwt);
ALTER TABLE licenses ADD COLUMN exp timestamp with time zone NOT NULL;
COMMENT ON COLUMN licenses.exp IS 'exp tracks the claim of the same name in the JWT, and we include it here so that we can easily query for licenses that have not yet expired.';

