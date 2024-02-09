-- name: GetWorkspaceAgentPortShare :one
SELECT * FROM workspace_agent_port_share WHERE workspace_id = $1 AND agent_name = $2 AND port = $3;

-- name: ListWorkspaceAgentPortShares :many
SELECT * FROM workspace_agent_port_share WHERE workspace_id = $1;

-- name: DeleteWorkspaceAgentPortShare :exec
DELETE FROM workspace_agent_port_share WHERE workspace_id = $1 AND agent_name = $2 AND port = $3;

-- name: UpsertWorkspaceAgentPortShare :one
INSERT INTO workspace_agent_port_share (workspace_id, agent_name, port, share_level)
VALUES ($1, $2, $3, $4)
ON CONFLICT (workspace_id, agent_name, port) DO UPDATE SET share_level = $4 RETURNING *;
