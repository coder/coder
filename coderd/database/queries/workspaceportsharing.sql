-- name: CreateWorkspacePortShareLevel :exec
INSERT INTO workspace_port_sharing (workspace_id, agent_name, port, share_level) VALUES ($1, $2, $3, $4);

-- name: GetWorkspacePortShareLevel :one
SELECT * FROM workspace_port_sharing WHERE workspace_id = $1 AND agent_name = $2 AND port = $3;

-- name: UpdateWorkspacePortShareLevel :exec
UPDATE workspace_port_sharing SET share_level = $1 WHERE workspace_id = $2 AND agent_name = $3 AND port = $4;

-- name: DeleteWorkspacePortShareLevel :exec
DELETE FROM workspace_port_sharing WHERE workspace_id = $1 AND agent_name = $2 AND port = $3;


