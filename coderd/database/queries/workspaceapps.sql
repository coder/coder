-- name: GetWorkspaceAppsByAgentID :many
SELECT * FROM workspace_apps WHERE agent_id = $1;

-- name: GetWorkspaceAppsByAgentIDs :many
SELECT * FROM workspace_apps WHERE agent_id = ANY(@ids :: uuid [ ]);

-- name: GetWorkspaceAppByAgentIDAndName :one
SELECT * FROM workspace_apps WHERE agent_id = $1 AND name = $2;

-- name: GetWorkspaceAppsCreatedAfter :many
SELECT * FROM workspace_apps WHERE created_at > $1;

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
        relative_path
    )
VALUES
    ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;
