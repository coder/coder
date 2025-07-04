-- Add purged_at column to workspace_builds table to track when logs/timings were purged
ALTER TABLE workspace_builds ADD COLUMN purged_at timestamptz;

-- Recreate the workspace_build_with_user view to include the new purged_at column
DROP VIEW workspace_build_with_user;
CREATE VIEW
        workspace_build_with_user
AS
SELECT
        workspace_builds.*,
        coalesce(visible_users.avatar_url, '') AS initiator_by_avatar_url,
        coalesce(visible_users.username, '') AS initiator_by_username,
        coalesce(visible_users.name, '') AS initiator_by_name
FROM
        workspace_builds
        LEFT JOIN
                visible_users
        ON
                workspace_builds.initiator_id = visible_users.id;

COMMENT ON VIEW workspace_build_with_user IS 'Joins in the username + avatar url of the initiated by user.';
