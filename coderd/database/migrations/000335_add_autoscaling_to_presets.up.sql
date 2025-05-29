-- Add autoscaling_timezone column to template_version_presets table
ALTER TABLE template_version_presets
ADD COLUMN autoscaling_timezone TEXT DEFAULT 'UTC' NOT NULL;

-- Add table for autoscaling schedules
CREATE TABLE template_version_preset_prebuild_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
    preset_id UUID NOT NULL,
    cron_expression TEXT NOT NULL,
    instances INTEGER NOT NULL,
    FOREIGN KEY (preset_id) REFERENCES template_version_presets (id) ON DELETE CASCADE
);
