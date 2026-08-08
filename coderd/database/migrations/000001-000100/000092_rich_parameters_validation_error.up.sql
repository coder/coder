ALTER TABLE template_version_parameters ADD COLUMN validation_error text NOT NULL DEFAULT '';

COMMENT ON COLUMN template_version_parameters.validation_error
IS 'Validation: error displayed when the regex does not match.';
