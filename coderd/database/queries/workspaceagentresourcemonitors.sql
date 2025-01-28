-- name: FetchAgentResourceMonitorsByAgentID :many
SELECT
	*
FROM
	workspace_agent_resource_monitors
WHERE
	agent_id = $1;

-- name: InsertWorkspaceAgentResourceMonitor :one
INSERT INTO
	workspace_agent_resource_monitors (
		agent_id,
		rtype,
		enabled,
		threshold,
		metadata,
		created_at
	)
VALUES
	($1, $2, $3, $4, $5, $6) RETURNING *;
