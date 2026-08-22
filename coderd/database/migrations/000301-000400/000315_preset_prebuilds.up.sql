ALTER TABLE template_version_presets
	ADD COLUMN desired_instances     INT NULL,
	ADD COLUMN invalidate_after_secs INT NULL DEFAULT 0;

-- Ensure that the idx_unique_preset_name index creation won't fail.
-- This is necessary because presets were released before the index was introduced,
-- so existing data might violate the uniqueness constraint.
WITH ranked AS (
    SELECT id, name, template_version_id,
           ROW_NUMBER() OVER (PARTITION BY name, template_version_id ORDER BY id) AS row_num
    FROM template_version_presets
)
UPDATE template_version_presets
SET name = ranked.name || '_auto_' || row_num
FROM ranked
WHERE template_version_presets.id = ranked.id AND row_num > 1;

-- We should not be able to have presets with the same name for a particular template version.
CREATE UNIQUE INDEX idx_unique_preset_name ON template_version_presets (name, template_version_id);
