ALTER TABLE workspace_build_parameters
ADD COLUMN IF NOT EXISTS sensitive boolean NOT NULL DEFAULT false,
ADD COLUMN IF NOT EXISTS value_key_id text REFERENCES dbcrypt_keys (active_key_digest);

COMMENT ON COLUMN workspace_build_parameters.sensitive IS 'Whether the value is sensitive. Sensitive values are encrypted when dbcrypt is enabled and are excluded from parameter insights. Set for ephemeral parameters when the build is created.';
COMMENT ON COLUMN workspace_build_parameters.value_key_id IS 'The ID of the key used to encrypt the value. If this is NULL, the value is not encrypted.';
