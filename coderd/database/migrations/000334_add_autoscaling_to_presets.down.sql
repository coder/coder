-- Drop the autoscaling schedules table
DROP TABLE template_version_preset_prebuild_schedules;

-- Remove added columns from template_version_presets table
ALTER TABLE template_version_presets
DROP COLUMN autoscaling_enabled,
DROP COLUMN autoscaling_timezone;
