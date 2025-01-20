CREATE TABLE template_version_presets
(
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	template_version_id UUID NOT NULL,
	-- TODO (sasswart): TEXT vs VARCHAR? Check with Mr Kopping.
	-- TODO (sasswart): A comment on the presets RFC mentioned that we may need to
	-- aggregate presets by name because we want statistics for related presets across
	-- template versions. This makes me uncomfortable. It couples constraints to a user
	-- facing field that could be avoided.
	name TEXT NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
	-- TODO (sasswart): What do we need updated_at for? Is it worth it to have a history table?
	-- TODO (sasswart): Will auditing have any relevance to presets?
	updated_at TIMESTAMP WITH TIME ZONE,
	FOREIGN KEY (template_version_id) REFERENCES template_versions (id) ON DELETE CASCADE
);

CREATE TABLE template_version_preset_parameters
(
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	template_version_preset_id UUID NOT NULL,
	name TEXT NOT NULL,
	-- TODO (sasswart): would it be beneficial to allow presets to still offer a choice for values?
	-- This would allow an operator to avoid having to create many similar templates where only one or
	-- a few values are different.
	value TEXT NOT NULL,
	FOREIGN KEY (template_version_preset_id) REFERENCES template_version_presets (id) ON DELETE CASCADE
);

ALTER TABLE workspace_builds
ADD COLUMN template_version_preset_id UUID NULL;

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
