ALTER TABLE ONLY provisioner_daemons
	ALTER COLUMN api_version SET DEFAULT 'v1.0'::text;
UPDATE provisioner_daemons
	SET api_version = 'v1.0'
	WHERE api_version = '1.0';
