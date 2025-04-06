-- name: InsertWorkspaceAgentDevcontainers :many
INSERT INTO
	workspace_agent_devcontainers (workspace_agent_id, created_at, id, name, workspace_folder, config_path)
SELECT
	@workspace_agent_id::uuid AS workspace_agent_id,
	@created_at::timestamptz AS created_at,
	unnest(@id::uuid[]) AS id,
	unnest(@name::text[]) AS name,
	unnest(@workspace_folder::text[]) AS workspace_folder,
	unnest(@config_path::text[]) AS config_path
RETURNING workspace_agent_devcontainers.*;

-- name: GetWorkspaceAgentDevcontainersByAgentID :many
SELECT
	*
FROM
	workspace_agent_devcontainers
WHERE
	workspace_agent_id = $1
ORDER BY
	created_at, id;
