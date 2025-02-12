ALTER TABLE template_version_presets
ALTER COLUMN id DROP DEFAULT;

ALTER TABLE template_version_preset_parameters
ALTER COLUMN id DROP DEFAULT;
