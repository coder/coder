ALTER TABLE ONLY provisioner_daemons
    ADD COLUMN api_version text NOT NULL DEFAULT '1.0';
COMMENT ON COLUMN provisioner_daemons.api_version IS 'The API version of the provisioner daemon';
