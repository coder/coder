-- TODO (sasswart): add IF NOT EXISTS and other clauses to make the migration more robust
CREATE TABLE template_version_presets
(
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	template_version_id UUID NOT NULL,
	name TEXT NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
	-- TODO (sasswart): Will auditing have any relevance to presets?
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

ALTER TABLE workspace_builds
ADD CONSTRAINT workspace_builds_template_version_preset_id_fkey
FOREIGN KEY (template_version_preset_id)
REFERENCES template_version_presets (id)
-- TODO (sasswart): SET NULL might not be the best choice here. The rest of the hierarchy has ON DELETE CASCADE.
-- We don't want CASCADE here, because we don't want to delete the workspace build if the preset is deleted.
-- However, do we want to lose record of the preset id for a workspace build?
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
