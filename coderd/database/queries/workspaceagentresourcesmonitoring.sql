-- name: FetchAgentResourcesMonitoringByAgentID :one
SELECT
	*
FROM
	agent_resources_monitoring
WHERE
	agent_id = $1;

-- name: InsertAgentResourcesMonitoring :one
INSERT INTO
	agent_resources_monitoring (
		agent_id,
		rtype,
		enabled,
		threshold,
		created_at,
		updated_at
	)
VALUES
	($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: FlushAgentResourcesMonitoringForAgentID :exec
DELETE FROM
	agent_resources_monitoring
WHERE
	agent_id = $1;
