ALTER TABLE mcp_server_configs
	ADD COLUMN signing_secret TEXT NOT NULL DEFAULT '',
	ADD COLUMN signing_secret_key_id TEXT REFERENCES dbcrypt_keys(active_key_digest);
