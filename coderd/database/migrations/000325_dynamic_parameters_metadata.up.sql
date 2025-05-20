ALTER TABLE template_version_terraform_values ADD COLUMN IF NOT EXISTS provisionerd_version TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN template_version_terraform_values.provisionerd_version IS
	'What version of the provisioning engine was used to generate the cached plan and module files.';
