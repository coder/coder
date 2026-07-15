ALTER TABLE mcp_server_user_tokens
	ADD COLUMN oauth_refresh_failure_reason TEXT NOT NULL DEFAULT ''
;

COMMENT ON COLUMN mcp_server_user_tokens.oauth_refresh_failure_reason IS
	'A permanent refresh failure (e.g. the upstream grant was revoked). Cached so we can avoid calling the provider again for the same error.'
;
