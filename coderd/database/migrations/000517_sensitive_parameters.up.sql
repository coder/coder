ALTER TABLE template_version_parameters
ADD COLUMN sensitive boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN template_version_parameters.sensitive
IS 'Sensitive parameter values are encrypted at rest and redacted when returned by the API.';

ALTER TABLE workspace_build_parameters
ADD COLUMN sensitive boolean NOT NULL DEFAULT false,
-- value_key_id references the dbcrypt key used to encrypt the value. When NULL
-- or empty, the value is stored in plaintext (encryption not configured).
-- NOTE: For production a foreign key to dbcrypt_keys(active_key_digest) should
-- be added so encryption keys cannot be revoked while still in use. It is
-- omitted here to keep the bulk array insert simple for the prototype.
ADD COLUMN value_key_id text;

COMMENT ON COLUMN workspace_build_parameters.sensitive
IS 'Sensitive parameter values are encrypted at rest and redacted when returned by the API.';
COMMENT ON COLUMN workspace_build_parameters.value_key_id
IS 'The ID of the dbcrypt key used to encrypt value. If NULL or empty, value is not encrypted.';
