ALTER TABLE template_versions
	ADD COLUMN git_auth_providers text[];

COMMENT ON COLUMN template_versions.git_auth_providers IS 'IDs of Git auth providers for a specific template version';
