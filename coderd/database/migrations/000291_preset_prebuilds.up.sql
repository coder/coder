CREATE TABLE template_version_preset_prebuilds
(
	id                    UUID PRIMARY KEY,
	preset_id             UUID NOT NULL,
	desired_instances     INT  NOT NULL,
	invalidate_after_secs INT  NULL DEFAULT 0,

	-- Deletion should never occur, but if we allow it we should check no prebuilds exist attached to this preset first.
	FOREIGN KEY (preset_id) REFERENCES template_version_presets (id) ON DELETE CASCADE
);

CREATE TABLE template_version_preset_prebuild_schedules
(
	id                 UUID PRIMARY KEY,
	preset_prebuild_id UUID NOT NULL,
	timezone           TEXT NOT NULL,
	cron_schedule      TEXT NOT NULL,
	desired_instances  INT  NOT NULL,

	FOREIGN KEY (preset_prebuild_id) REFERENCES template_version_preset_prebuilds (id) ON DELETE CASCADE
);
