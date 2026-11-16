ALTER TABLE template_version_terraform_values
ADD COLUMN IF NOT EXISTS cached_plan_key_id text REFERENCES dbcrypt_keys (active_key_digest);

COMMENT ON COLUMN template_version_terraform_values.cached_plan_key_id IS 'The ID of the key used to encrypt the cached plan. If this is NULL, the plan is not encrypted. Cached Terraform plans embed data source values such as workspace owner credentials and template variable values (SEC-1491).';
