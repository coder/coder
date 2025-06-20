-- Drop the prebuild schedules table
DROP TABLE template_version_preset_prebuild_schedules;

-- Remove scheduling_timezone column from template_version_presets table
ALTER TABLE template_version_presets
DROP COLUMN scheduling_timezone;
