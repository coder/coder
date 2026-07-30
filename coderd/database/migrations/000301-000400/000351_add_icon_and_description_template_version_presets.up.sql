ALTER TABLE template_version_presets
	ADD COLUMN IF NOT EXISTS description VARCHAR(128) NOT NULL DEFAULT '',
	ADD COLUMN IF NOT EXISTS icon VARCHAR(256) NOT NULL DEFAULT '';

COMMENT ON COLUMN template_version_presets.description IS 'Short text describing the preset (max 128 characters).';
COMMENT ON COLUMN template_version_presets.icon IS 'URL or path to an icon representing the preset (max 256 characters).';
