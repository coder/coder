ALTER TABLE external_auth_links
	ADD COLUMN oauth_refresh_failure_reason TEXT NOT NULL DEFAULT ''
;

COMMENT ON COLUMN external_auth_links.oauth_refresh_failure_reason IS
	'This error means the refresh token is invalid. Cached so we can avoid calling the external provider again for the same error.'
;
