ALTER TABLE template_version_presets
    ADD COLUMN desired_instances     INT NULL,
    ADD COLUMN invalidate_after_secs INT NULL DEFAULT 0;

-- We should not be able to have presets with the same name for a particular template version.
CREATE UNIQUE INDEX idx_unique_preset_name ON template_version_presets (name, template_version_id);
