ALTER TABLE external_auth_links
ADD COLUMN IF NOT EXISTS oauth_extra_key_id text REFERENCES dbcrypt_keys (active_key_digest);

COMMENT ON COLUMN external_auth_links.oauth_extra_key_id IS 'The ID of the key used to encrypt the OAuth extra data. If this is NULL, the extra data is not encrypted.';
