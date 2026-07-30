ALTER TABLE template_version_presets
ALTER COLUMN id SET DEFAULT gen_random_uuid();

ALTER TABLE template_version_preset_parameters
ALTER COLUMN id SET DEFAULT gen_random_uuid();
