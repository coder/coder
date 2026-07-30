ALTER TABLE template_version_parameters ADD COLUMN ephemeral boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN template_version_parameters.ephemeral
IS 'The value of an ephemeral parameter will not be preserved between consecutive workspace builds.';
