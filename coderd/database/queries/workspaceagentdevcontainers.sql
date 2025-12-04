-- name: InsertWorkspaceAgentDevcontainers :many
INSERT INTO
	workspace_agent_devcontainers (workspace_agent_id, created_at, id, name, workspace_folder, config_path, build_cache_from)
SELECT
	@workspace_agent_id::uuid AS workspace_agent_id,
	@created_at::timestamptz AS created_at,
	unnest(@id::uuid[]) AS id,
	unnest(@name::text[]) AS name,
	unnest(@workspace_folder::text[]) AS workspace_folder,
	unnest(@config_path::text[]) AS config_path,
	unnest(@build_cache_from::text[][]) AS build_cache_from
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
