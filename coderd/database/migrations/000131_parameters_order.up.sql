ALTER TABLE template_version_parameters ADD COLUMN priority integer NOT NULL DEFAULT 0;

COMMENT ON COLUMN template_version_parameters.priority
IS 'Display priority';
