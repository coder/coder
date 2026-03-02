ALTER TABLE user_links
	ADD COLUMN token_updated timestamp with time zone
		NOT NULL
		DEFAULT '0001-01-01 00:00:00+00'::timestamp with time zone;

COMMENT ON COLUMN user_links.token_updated IS
	'Should match whenever oauth_access_token is updated to a new token.';
