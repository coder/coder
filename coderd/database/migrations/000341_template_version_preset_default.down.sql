DROP INDEX IF EXISTS idx_template_version_presets_default;
ALTER TABLE template_version_presets DROP COLUMN IF EXISTS is_default;