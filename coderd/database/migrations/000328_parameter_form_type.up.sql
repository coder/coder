-- Intentionally leaving the default blank. The provisioner will not re-run any
-- imports to backfill these values. Missing values just have to be handled.
ALTER TABLE template_version_parameters ADD COLUMN form_type text NOT NULL DEFAULT '';

COMMENT ON COLUMN template_version_parameters.form_type
	IS 'Specify what form_type should be used to render the parameter in the UI. This value should correspond to an enum, but this will not be enforced in the sql. Mistakes here should not be fatal for functional usage.';
