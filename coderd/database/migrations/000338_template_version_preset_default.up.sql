ALTER TABLE template_version_presets ADD COLUMN is_default BOOLEAN NOT NULL DEFAULT FALSE;

-- Add a unique constraint to ensure only one default preset per template version
CREATE UNIQUE INDEX idx_template_version_presets_default 
ON template_version_presets (template_version_id) 
WHERE is_default = TRUE;