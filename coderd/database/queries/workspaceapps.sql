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
WHERE
    -- Prevent cross-tenant/cross-workspace agent rebinding (SEC-91).
    -- App IDs persist across builds of the same workspace, but agent IDs are
    -- regenerated every build, so compare by the workspace that owns the agent
    -- rather than by agent_id. Permit unowned apps to be claimed and permit
    -- same-workspace rebuilds. If an existing app belongs to a workspace, block
    -- moves to both different workspaces and template import or dry-run agents
    -- that resolve to no workspace. The conflicting row is then left untouched,
    -- and the :one query returns no row, which the caller treats as a
    -- rejection.
    NOT EXISTS (
        SELECT 1
        FROM workspace_agents AS existing_agent
        INNER JOIN workspace_resources AS existing_resource
            ON existing_agent.resource_id = existing_resource.id
        INNER JOIN workspace_builds AS existing_build
            ON existing_resource.job_id = existing_build.job_id
        WHERE existing_agent.id = workspace_apps.agent_id
    )
    OR EXISTS (
        SELECT 1
        FROM workspace_agents AS existing_agent
        INNER JOIN workspace_resources AS existing_resource
            ON existing_agent.resource_id = existing_resource.id
        INNER JOIN workspace_builds AS existing_build
            ON existing_resource.job_id = existing_build.job_id
        INNER JOIN workspace_agents AS incoming_agent
            ON incoming_agent.id = EXCLUDED.agent_id
        INNER JOIN workspace_resources AS incoming_resource
            ON incoming_agent.resource_id = incoming_resource.id
        INNER JOIN workspace_builds AS incoming_build
            ON incoming_resource.job_id = incoming_build.job_id
        WHERE
            existing_agent.id = workspace_apps.agent_id
            AND existing_build.workspace_id = incoming_build.workspace_id
    )
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
-- id DESC is a stability tiebreaker, not an insertion-order signal: back-to-back
-- inserts can share a created_at on platforms with coarse time.Now() resolution,
-- and id is a random UUID, so this only guarantees a deterministic pick, not the
-- later row. Callers must not depend on sub-microsecond recency here.
SELECT DISTINCT ON (workspace_id)
  *
FROM workspace_app_statuses
WHERE workspace_id = ANY(@ids :: uuid[])
ORDER BY workspace_id, created_at DESC, id DESC;

