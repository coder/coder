ALTER TABLE template_version_parameters
ADD COLUMN sensitive boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN template_version_parameters.sensitive
IS 'Sensitive parameter values are encrypted at rest and redacted when returned by the API.';

ALTER TABLE workspace_build_parameters
ADD COLUMN sensitive boolean NOT NULL DEFAULT false,
-- value_key_id references the dbcrypt key used to encrypt the value. When NULL,
-- the value is stored in plaintext (encryption not configured).
ADD COLUMN value_key_id text;

-- The foreign key ensures an encryption key cannot be revoked while any
-- parameter value still references it. Non-encrypted rows store NULL.
ALTER TABLE ONLY workspace_build_parameters
ADD CONSTRAINT workspace_build_parameters_value_key_id_fkey
FOREIGN KEY (value_key_id) REFERENCES dbcrypt_keys(active_key_digest);

COMMENT ON COLUMN workspace_build_parameters.sensitive
IS 'Sensitive parameter values are encrypted at rest and redacted when returned by the API.';
COMMENT ON COLUMN workspace_build_parameters.value_key_id
IS 'The ID of the dbcrypt key used to encrypt value. If NULL, value is not encrypted.';
