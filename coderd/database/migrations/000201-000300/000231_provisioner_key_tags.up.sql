ALTER TABLE provisioner_keys ADD COLUMN tags jsonb DEFAULT '{}'::jsonb NOT NULL;
ALTER TABLE provisioner_keys ALTER COLUMN tags DROP DEFAULT;
