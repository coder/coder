ALTER TABLE workspace_build_parameters
ADD COLUMN IF NOT EXISTS sensitive boolean NOT NULL DEFAULT false,
ADD COLUMN IF NOT EXISTS value_key_id text REFERENCES dbcrypt_keys (active_key_digest);

COMMENT ON COLUMN workspace_build_parameters.sensitive IS 'Sensitive parameter values (currently the values of ephemeral parameters) are encrypted at rest when dbcrypt is enabled and are excluded from parameter insights and workspace search (SEC-1491).';
COMMENT ON COLUMN workspace_build_parameters.value_key_id IS 'The ID of the key used to encrypt the value. If this is NULL, the value is not encrypted.';

-- Mark existing values of ephemeral parameters as sensitive so
-- `coder server dbcrypt rotate` encrypts historic rows too.
UPDATE workspace_build_parameters wbp
SET sensitive = true
FROM workspace_builds wb
JOIN template_version_parameters tvp ON tvp.template_version_id = wb.template_version_id
WHERE wb.id = wbp.workspace_build_id
	AND tvp.name = wbp.name
	AND tvp.ephemeral;
