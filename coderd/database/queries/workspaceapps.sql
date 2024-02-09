-- name: GetWorkspaceAppsByAgentID :many
SELECT * FROM workspace_apps WHERE agent_id = $1 ORDER BY slug ASC;

-- name: GetWorkspaceAppsByAgentIDs :many
SELECT * FROM workspace_apps WHERE agent_id = ANY(@ids :: uuid [ ]) ORDER BY slug ASC;

-- name: GetWorkspaceAppByAgentIDAndSlug :one
SELECT * FROM workspace_apps WHERE agent_id = $1 AND slug = $2;

-- name: GetWorkspaceAppsCreatedAfter :many
SELECT * FROM workspace_apps WHERE created_at > $1 ORDER BY slug ASC;

-- name: InsertWorkspaceApp :one
INSERT INTO
    workspace_apps (
        id,
        created_at,
        agent_id,
        slug,
        display_name,
        icon,
        command,
        url,
        external,
        subdomain,
        sharing_level,
        healthcheck_url,
        healthcheck_interval,
        healthcheck_threshold,
        health,
        display_order
    )
VALUES
    ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16) RETURNING *;

-- name: UpdateWorkspaceAppHealthByID :exec
UPDATE
	workspace_apps
SET
	health = $2
WHERE
	id = $1;
