CREATE TABLE template_version_preset_prebuilds
(
	id                    UUID PRIMARY KEY,
	preset_id             UUID NOT NULL,
	desired_instances     INT  NOT NULL,
	invalidate_after_secs INT  NULL DEFAULT 0,

	-- Deletion should never occur, but if we allow it we should check no prebuilds exist attached to this preset first.
	FOREIGN KEY (preset_id) REFERENCES template_version_presets (id) ON DELETE CASCADE
);

-- We should not be able to have presets with the same name for a particular template version.
CREATE UNIQUE INDEX idx_unique_preset_name ON template_version_presets (name, template_version_id);
