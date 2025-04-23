ALTER TABLE template_version_terraform_values
	ADD COLUMN IF NOT EXISTS tfstate bytea DEFAULT null;

COMMENT ON COLUMN template_version_terraform_values.tfstate IS 'Tarball of the relevant tfstate directory files for dynamic parameters. Not all files are included.'
