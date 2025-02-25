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
		state,
		threshold,
		created_at,
		updated_at,
		debounced_until
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: InsertVolumeResourceMonitor :one
INSERT INTO
	workspace_agent_volume_resource_monitors (
		agent_id,
		path,
		enabled,
		state,
		threshold,
		created_at,
		updated_at,
		debounced_until
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: UpdateMemoryResourceMonitor :exec
UPDATE workspace_agent_memory_resource_monitors
SET
	updated_at = $2,
	state = $3,
	debounced_until = $4
WHERE
	agent_id = $1;

-- name: UpdateVolumeResourceMonitor :exec
UPDATE workspace_agent_volume_resource_monitors
SET
		updated_at = $3,
		state = $4,
		debounced_until = $5
WHERE
		agent_id = $1 AND path = $2;
