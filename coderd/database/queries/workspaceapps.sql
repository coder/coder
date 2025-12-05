-- name: GetWorkspaceAppsByAgentID :many
SELECT * FROM workspace_apps WHERE agent_id = $1 ORDER BY slug ASC;

-- name: GetWorkspaceAppsByAgentIDs :many
SELECT * FROM workspace_apps WHERE agent_id = ANY(@ids :: uuid [ ]) ORDER BY slug ASC;

-- name: GetWorkspaceAppByAgentIDAndSlug :one
SELECT * FROM workspace_apps WHERE agent_id = $1 AND slug = $2;

-- name: GetWorkspaceAppsCreatedAfter :many
SELECT * FROM workspace_apps WHERE created_at > $1 ORDER BY slug ASC;

-- name: UpsertWorkspaceApp :one
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
        display_order,
        hidden,
        open_in,
        display_group,
        tooltip
    )
VALUES
    ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
ON CONFLICT (id) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    icon = EXCLUDED.icon,
    command = EXCLUDED.command,
    url = EXCLUDED.url,
    external = EXCLUDED.external,
    subdomain = EXCLUDED.subdomain,
    sharing_level = EXCLUDED.sharing_level,
    healthcheck_url = EXCLUDED.healthcheck_url,
    healthcheck_interval = EXCLUDED.healthcheck_interval,
    healthcheck_threshold = EXCLUDED.healthcheck_threshold,
    health = EXCLUDED.health,
    display_order = EXCLUDED.display_order,
    hidden = EXCLUDED.hidden,
    open_in = EXCLUDED.open_in,
    display_group = EXCLUDED.display_group,
    agent_id = EXCLUDED.agent_id,
    slug = EXCLUDED.slug,
    tooltip = EXCLUDED.tooltip
RETURNING *;

-- name: UpdateWorkspaceAppHealthByID :exec
UPDATE
	workspace_apps
SET
	health = $2
WHERE
	id = $1;

-- name: InsertWorkspaceAppStatus :one
INSERT INTO workspace_app_statuses (id, created_at, workspace_id, agent_id, app_id, state, message, uri)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetWorkspaceAppStatusesByAppIDs :many
SELECT * FROM workspace_app_statuses WHERE app_id = ANY(@ids :: uuid [ ])
ORDER BY created_at DESC, id DESC;

-- name: GetLatestWorkspaceAppStatusByAppID :one
SELECT *
FROM workspace_app_statuses
WHERE app_id = @app_id::uuid
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: GetLatestWorkspaceAppStatusesByWorkspaceIDs :many
SELECT DISTINCT ON (workspace_id)
  *
FROM workspace_app_statuses
WHERE workspace_id = ANY(@ids :: uuid[])
ORDER BY workspace_id, created_at DESC;
