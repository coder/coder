DROP INDEX IF EXISTS idx_provisioner_daemons_name_owner_key;

ALTER TABLE ONLY provisioner_daemons
	ADD CONSTRAINT provisioner_daemons_name_key UNIQUE (name);
