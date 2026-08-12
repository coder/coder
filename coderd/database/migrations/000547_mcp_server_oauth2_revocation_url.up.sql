ALTER TABLE mcp_server_configs
	ADD COLUMN oauth2_revocation_url text NOT NULL DEFAULT '';
