CREATE VIEW workspace_build_deadlines AS
	SELECT 
		workspace_builds.id, 
		LEAST(
			workspaces.last_used_at + (workspaces.ttl / 1000 / 1000 / 1000 || ' seconds')::interval,
			workspace_builds.max_deadline
		)::timestamp with time zone AS deadline
FROM 
	workspace_builds 
LEFT JOIN 
	workspaces 
ON 
	workspace_builds.workspace_id = workspaces.id;
