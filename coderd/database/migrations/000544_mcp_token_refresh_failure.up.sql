ALTER TABLE mcp_server_user_tokens
	ADD COLUMN oauth_refresh_failure_reason TEXT NOT NULL DEFAULT ''
;
