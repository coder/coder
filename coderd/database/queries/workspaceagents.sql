-- name: GetWorkspaceAgentByAuthToken :one
SELECT
	*
FROM
	workspace_agents
WHERE
	auth_token = $1
ORDER BY
	created_at DESC;

-- name: GetWorkspaceAgentByInstanceID :one
SELECT
	*
FROM
	workspace_agents
WHERE
	auth_instance_id = @auth_instance_id :: TEXT
ORDER BY
	created_at DESC;

-- name: GetWorkspaceAgentByResourceID :one
SELECT
	*
FROM
	workspace_agents
WHERE
	resource_id = $1;

-- name: InsertWorkspaceAgent :one
INSERT INTO
	workspace_agents (
		id,
		created_at,
		updated_at,
		resource_id,
		auth_token,
		auth_instance_id,
		environment_variables,
		startup_script,
		instance_metadata,
		resource_metadata
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING *;

-- name: UpdateWorkspaceAgentConnectionByID :exec
UPDATE
	workspace_agents
SET
	first_connected_at = $2,
	last_connected_at = $3,
	disconnected_at = $4
WHERE
	id = $1;
