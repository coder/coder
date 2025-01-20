-- Recreate the view to exclude the new column.
DROP VIEW workspace_build_with_user;

ALTER TABLE workspace_builds
DROP COLUMN template_version_preset_id;

DROP TABLE template_version_preset_parameters;

DROP TABLE template_version_presets;

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
