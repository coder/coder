ALTER TABLE template_version_parameters ADD COLUMN display_order integer NOT NULL DEFAULT 0;

COMMENT ON COLUMN template_version_parameters.display_order
IS 'Specifies the order in which to display parameters in user interfaces.';
