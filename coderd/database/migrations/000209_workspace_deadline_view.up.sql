CREATE VIEW workspace_deadlines AS
	SELECT 
		workspaces.id, 
		LEAST(
			workspaces.last_used_at + (workspaces.ttl / 1000 / 1000 / 1000 || ' seconds')::interval,
			workspace_builds.max_deadline
		) AS deadline
FROM 
	workspaces
LEFT JOIN 
	workspace_builds
ON 
	workspace_builds.workspace_id = workspaces.id
WHERE
    workspace_builds.build_number = (
		SELECT
			MAX(build_number)
		FROM
			workspace_builds
		WHERE
			workspace_builds.workspace_id = workspaces.id
	);

ALTER TABLE workspace_builds RENAME COLUMN deadline TO deadline_deprecated;

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
