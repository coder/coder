-- name: GetWorkspaceMonitor :one
SELECT *
FROM workspace_monitors
WHERE
	workspace_id = $1 AND
	monitor_type = $2 AND
	volume_path IS NOT DISTINCT FROM $3;

-- name: InsertWorkspaceMonitor :one
INSERT INTO workspace_monitors (
	workspace_id,
	monitor_type,
	volume_path,
	state,
	created_at,
	updated_at,
	debounced_until
) VALUES (
	$1,
	$2,
	$3,
	$4,
	$5,
	$6,
	$7
) RETURNING *;

-- name: UpdateWorkspaceMonitor :exec
UPDATE workspace_monitors
SET
	state = $4,
	updated_at = $5,
	debounced_until = $6
WHERE
	workspace_id = $1 AND
	monitor_type = $2 AND
	volume_path IS NOT DISTINCT FROM $3;
