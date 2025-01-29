-- name: FetchMemoryResourceMonitorsByAgentID :one
SELECT
	*
FROM
	workspace_agent_memory_resource_monitors
WHERE
	agent_id = $1;

-- name: FetchVolumesResourceMonitorsByAgentID :many
SELECT
	*
FROM
	workspace_agent_volume_resource_monitors
WHERE
	agent_id = $1;

-- name: InsertMemoryResourceMonitor :one
INSERT INTO
	workspace_agent_memory_resource_monitors (
		agent_id,
		enabled,
		threshold,
		created_at
	)
VALUES
	($1, $2, $3, $4) RETURNING *;

-- name: InsertVolumeResourceMonitor :one
INSERT INTO
	workspace_agent_volume_resource_monitors (
		agent_id,
		path,
		enabled,
		threshold,
		created_at
	)
VALUES
	($1, $2, $3, $4, $5) RETURNING *;
