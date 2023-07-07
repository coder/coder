-- Unfortunately we can't bring back deleted values.

ALTER TABLE template_version_parameters ADD COLUMN legacy_variable_name text NOT NULL DEFAULT '';

COMMENT ON COLUMN template_version_parameters.legacy_variable_name IS 'Name of the legacy variable for migration purposes';
