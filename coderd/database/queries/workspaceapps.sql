-- name: GetWorkspaceAppsByAgentID :many
SELECT * FROM workspace_apps WHERE agent_id = $1 ORDER BY name ASC;

-- name: GetWorkspaceAppsByAgentIDs :many
SELECT * FROM workspace_apps WHERE agent_id = ANY(@ids :: uuid [ ]) ORDER BY name ASC;

-- name: GetWorkspaceAppByAgentIDAndName :one
SELECT * FROM workspace_apps WHERE agent_id = $1 AND name = $2;

-- name: GetWorkspaceAppsCreatedAfter :many
SELECT * FROM workspace_apps WHERE created_at > $1 ORDER BY name ASC;

-- name: InsertWorkspaceApp :one
INSERT INTO
    workspace_apps (
        id,
        created_at,
        agent_id,
        name,
        icon,
        command,
        url,
        relative_path,
		healthcheck_enabled,
		healthcheck_period,
		healthcheck_threshold,
		health
    )
VALUES
    ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING *;

-- name: UpdateWorkspaceAppHealthByID :exec
UPDATE
	workspace_apps
SET
	updated_at = $2,
	health = $3
WHERE
	id = $1;
