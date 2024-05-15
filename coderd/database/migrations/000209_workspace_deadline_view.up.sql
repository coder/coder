CREATE VIEW workspace_build_deadlines AS
	SELECT 
		workspace_builds.id, 
		COALESCE(
			LEAST(
				workspaces.last_used_at + (workspaces.ttl / 1000 / 1000 / 1000 || ' seconds')::interval,
				workspace_builds.max_deadline
			),
			'0001-01-01 00:00:00+00'::timestamp with time zone
		) AS deadline
FROM 
	workspace_builds 
LEFT JOIN 
	workspaces 
ON 
	workspace_builds.workspace_id = workspaces.id;
