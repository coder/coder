ALTER TABLE template_version_parameters ADD COLUMN display_name text NOT NULL DEFAULT '';

COMMENT ON COLUMN template_version_parameters.display_name
IS 'Display name of the rich parameter';
