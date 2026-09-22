ALTER TABLE template_version_parameters ADD COLUMN sensitive boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN template_version_parameters.sensitive
IS 'Sensitive parameters have their values redacted in logs, insights, notifications, and the API. Values are still stored in the database and in Terraform state.';
