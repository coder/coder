ALTER TABLE template_version_parameters ADD COLUMN required boolean NOT NULL DEFAULT true; -- default: true, as so far every parameter should be marked as required

COMMENT ON COLUMN template_version_parameters.required IS 'Is parameter required?';
