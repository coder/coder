ALTER TABLE template_version_presets
	DROP COLUMN desired_instances,
	DROP COLUMN invalidate_after_secs;

DROP INDEX IF EXISTS idx_unique_preset_name;
