DROP INDEX IF EXISTS idx_provisioner_daemons_name_owner_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_provisioner_daemons_name_owner_key
	ON provisioner_daemons
	USING btree (name, LOWER(COALESCE(tags->>'owner', '')::text));

COMMENT ON INDEX idx_provisioner_daemons_name_owner_key
	IS 'Allow unique provisioner daemon names by user';
