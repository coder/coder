-- name: GetWorkspaceAgentPluginsByAgentID :many
SELECT *
FROM workspace_agent_plugins
WHERE agent_id = $1
ORDER BY slug;

-- name: GetWorkspaceAgentPluginsByAgentIDs :many
SELECT *
FROM workspace_agent_plugins
WHERE agent_id = ANY(@agent_ids::uuid[])
ORDER BY agent_id, slug;

-- name: GetWorkspaceAgentPluginByAgentIDAndSlug :one
SELECT *
FROM workspace_agent_plugins
WHERE agent_id = $1 AND slug = $2;

-- name: UpsertWorkspaceAgentPlugin :one
INSERT INTO workspace_agent_plugins (
    id,
    created_at,
    agent_id,
    slug,
    display_name,
    icon,
    url,
    backend_entry
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8
) ON CONFLICT (agent_id, slug)
DO UPDATE SET
    display_name = EXCLUDED.display_name,
    icon = EXCLUDED.icon,
    url = EXCLUDED.url,
    backend_entry = EXCLUDED.backend_entry
RETURNING *;
