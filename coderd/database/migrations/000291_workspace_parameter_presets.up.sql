CREATE TABLE template_version_presets
(
	id UUID PRIMARY KEY NOT NULL,
	template_version_id UUID NOT NULL,
	name TEXT NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (template_version_id) REFERENCES template_versions (id) ON DELETE CASCADE
);

CREATE TABLE template_version_preset_parameters
(
	id UUID PRIMARY KEY NOT NULL,
	template_version_preset_id UUID NOT NULL,
	name TEXT NOT NULL,
	value TEXT NOT NULL,
	FOREIGN KEY (template_version_preset_id) REFERENCES template_version_presets (id) ON DELETE CASCADE
);

ALTER TABLE workspace_builds
ADD COLUMN template_version_preset_id UUID NULL;

ALTER TABLE workspace_builds
ADD CONSTRAINT workspace_builds_template_version_preset_id_fkey
FOREIGN KEY (template_version_preset_id)
REFERENCES template_version_presets (id)
ON DELETE SET NULL;

-- Recreate the view to include the new column.
DROP VIEW workspace_build_with_user;
CREATE VIEW
	workspace_build_with_user
AS
SELECT
	workspace_builds.*,
	coalesce(visible_users.avatar_url, '') AS initiator_by_avatar_url,
	coalesce(visible_users.username, '') AS initiator_by_username
FROM
	workspace_builds
	LEFT JOIN
		visible_users
	ON
		workspace_builds.initiator_id = visible_users.id;

COMMENT ON VIEW workspace_build_with_user IS 'Joins in the username + avatar url of the initiated by user.';
