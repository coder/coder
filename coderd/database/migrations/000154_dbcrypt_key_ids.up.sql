CREATE TABLE IF NOT EXISTS dbcrypt_keys (
	number int NOT NULL PRIMARY KEY,
	active_key_digest text UNIQUE,
	revoked_key_digest text UNIQUE,
	created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
	revoked_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
	test TEXT NOT NULL
);

COMMENT ON TABLE dbcrypt_keys IS 'A table used to store the keys used to encrypt the database.';
COMMENT ON COLUMN dbcrypt_keys.number IS 'An integer used to identify the key.';
COMMENT ON COLUMN dbcrypt_keys.active_key_digest IS 'If the key is active, the digest of the active key.';
COMMENT ON COLUMN dbcrypt_keys.revoked_key_digest IS 'If the key has been revoked, the digest of the revoked key.';
COMMENT ON COLUMN dbcrypt_keys.created_at IS 'The time at which the key was created.';
COMMENT ON COLUMN dbcrypt_keys.revoked_at IS 'The time at which the key was revoked.';
COMMENT ON COLUMN dbcrypt_keys.test IS 'A column used to test the encryption.';

ALTER TABLE git_auth_links
ADD COLUMN IF NOT EXISTS oauth_access_token_key_id text REFERENCES dbcrypt_keys(active_key_digest),
ADD COLUMN IF NOT EXISTS oauth_refresh_token_key_id text REFERENCES dbcrypt_keys(active_key_digest);

COMMENT ON COLUMN git_auth_links.oauth_access_token_key_id IS 'The ID of the key used to encrypt the OAuth access token. If this is NULL, the access token is not encrypted';
COMMENT ON COLUMN git_auth_links.oauth_refresh_token_key_id IS 'The ID of the key used to encrypt the OAuth refresh token. If this is NULL, the refresh token is not encrypted';

ALTER TABLE user_links
ADD COLUMN IF NOT EXISTS oauth_access_token_key_id text REFERENCES dbcrypt_keys(active_key_digest),
ADD COLUMN IF NOT EXISTS oauth_refresh_token_key_id text REFERENCES dbcrypt_keys(active_key_digest);

COMMENT ON COLUMN user_links.oauth_access_token_key_id IS 'The ID of the key used to encrypt the OAuth access token. If this is NULL, the access token is not encrypted';
COMMENT ON COLUMN user_links.oauth_refresh_token_key_id IS 'The ID of the key used to encrypt the OAuth refresh token. If this is NULL, the refresh token is not encrypted';
