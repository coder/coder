ALTER TABLE template_version_variables
ADD COLUMN IF NOT EXISTS value_key_id text REFERENCES dbcrypt_keys (active_key_digest);

COMMENT ON COLUMN template_version_variables.value_key_id IS 'The ID of the key used to encrypt the value. If this is NULL, the value is not encrypted. Only values of variables marked sensitive are encrypted.';
