CREATE TYPE parameter_form_type AS ENUM ('', 'error', 'radio', 'dropdown', 'input', 'textarea', 'slider', 'checkbox', 'switch', 'tag-select', 'multi-select');
COMMENT ON TYPE parameter_form_type
	IS 'Enum set should match the terraform provider set. This is defined as future form_types are not supported, and should be rejected. '
	'Always include the empty string for using the default form type.';

-- Intentionally leaving the default blank. The provisioner will not re-run any
-- imports to backfill these values. Missing values just have to be handled.
ALTER TABLE template_version_parameters ADD COLUMN form_type parameter_form_type NOT NULL DEFAULT '';

COMMENT ON COLUMN template_version_parameters.form_type
	IS 'Specify what form_type should be used to render the parameter in the UI. Unsupported values are rejected.';
