DROP INDEX IF EXISTS idx_provisioner_daemons_name_owner_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_provisioner_daemons_name_owner_key
	ON provisioner_daemons
	USING btree (name, lower((tags->>'owner')::text));

COMMENT ON INDEX idx_provisioner_daemons_name_owner_key
	IS 'Relax uniqueness constraint for provisioner daemon names';
