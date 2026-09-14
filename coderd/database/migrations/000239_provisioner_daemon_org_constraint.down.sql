CREATE UNIQUE INDEX idx_provisioner_daemons_name_owner_key ON provisioner_daemons USING btree (name, lower(COALESCE((tags ->> 'owner'::text), ''::text)));

COMMENT ON INDEX idx_provisioner_daemons_name_owner_key IS 'Allow unique provisioner daemon names by user';

DROP INDEX idx_provisioner_daemons_org_name_owner_key;
