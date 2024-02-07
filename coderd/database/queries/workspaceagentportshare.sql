-- name: InsertWorkspaceAgentPortShare :one
INSERT INTO workspace_agent_port_share (workspace_id, agent_name, port, share_level) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: GetWorkspaceAgentPortShare :one
SELECT * FROM workspace_agent_port_share WHERE workspace_id = $1 AND agent_name = $2 AND port = $3;

-- name: ListWorkspaceAgentPortShares :many
SELECT * FROM workspace_agent_port_share WHERE workspace_id = $1;

-- name: UpdateWorkspaceAgentPortShare :exec
UPDATE workspace_agent_port_share SET share_level = $1 WHERE workspace_id = $2 AND agent_name = $3 AND port = $4;

-- name: DeleteWorkspaceAgentPortShare :exec
DELETE FROM workspace_agent_port_share WHERE workspace_id = $1 AND agent_name = $2 AND port = $3;
