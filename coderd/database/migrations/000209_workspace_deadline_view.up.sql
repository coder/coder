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
