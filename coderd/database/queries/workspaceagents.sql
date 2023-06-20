-- name: GetWorkspaceAgentByAuthToken :one
SELECT
	*
FROM
	workspace_agents
WHERE
	auth_token = $1
ORDER BY
	created_at DESC;

-- name: GetWorkspaceAgentByID :one
SELECT
	*
FROM
	workspace_agents
WHERE
	id = $1;

-- name: GetWorkspaceAgentByInstanceID :one
SELECT
	*
FROM
	workspace_agents
WHERE
	auth_instance_id = @auth_instance_id :: TEXT
ORDER BY
	created_at DESC;

-- name: GetWorkspaceAgentsByResourceIDs :many
SELECT
	*
FROM
	workspace_agents
WHERE
	resource_id = ANY(@ids :: uuid [ ]);

-- name: GetWorkspaceAgentsCreatedAfter :many
SELECT * FROM workspace_agents WHERE created_at > $1;

-- name: InsertWorkspaceAgent :one
INSERT INTO
	workspace_agents (
		id,
		created_at,
		updated_at,
		name,
		resource_id,
		auth_token,
		auth_instance_id,
		architecture,
		environment_variables,
		operating_system,
		startup_script,
		directory,
		instance_metadata,
		resource_metadata,
		connection_timeout_seconds,
		troubleshooting_url,
		motd_file,
		startup_script_behavior,
		startup_script_timeout_seconds,
		shutdown_script,
		shutdown_script_timeout_seconds
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21) RETURNING *;

-- name: UpdateWorkspaceAgentConnectionByID :exec
UPDATE
	workspace_agents
SET
	first_connected_at = $2,
	last_connected_at = $3,
	last_connected_replica_id = $4,
	disconnected_at = $5,
	updated_at = $6
WHERE
	id = $1;

-- name: UpdateWorkspaceAgentStartupByID :exec
UPDATE
	workspace_agents
SET
	version = $2,
	expanded_directory = $3,
	subsystem = $4
WHERE
	id = $1;

-- name: GetWorkspaceAgentLifecycleStateByID :one
SELECT
	lifecycle_state,
	started_at,
	ready_at
FROM
	workspace_agents
WHERE
	id = $1;


-- name: UpdateWorkspaceAgentLifecycleStateByID :exec
UPDATE
	workspace_agents
SET
	lifecycle_state = $2,
	started_at = $3,
	ready_at = $4
WHERE
	id = $1;

-- name: InsertWorkspaceAgentMetadata :exec
INSERT INTO
	workspace_agent_metadata (
		workspace_agent_id,
		display_name,
		key,
		script,
		timeout,
		interval
	)
VALUES
	($1, $2, $3, $4, $5, $6);

-- name: UpdateWorkspaceAgentMetadata :exec
UPDATE
	workspace_agent_metadata
SET
	value = $3,
	error = $4,
	collected_at = $5
WHERE
	workspace_agent_id = $1
	AND key = $2;

-- name: GetWorkspaceAgentMetadata :many
SELECT
	*
FROM
	workspace_agent_metadata
WHERE
	workspace_agent_id = $1;

-- name: UpdateWorkspaceAgentStartupLogOverflowByID :exec
UPDATE
	workspace_agents
SET
	startup_logs_overflowed = $2
WHERE
	id = $1;

-- name: GetWorkspaceAgentStartupLogsAfter :many
SELECT
	*
FROM
	workspace_agent_startup_logs
WHERE
	agent_id = $1
	AND (
		id > @created_after
	) ORDER BY id ASC;

-- name: InsertWorkspaceAgentStartupLogs :many
WITH new_length AS (
	UPDATE workspace_agents SET
	startup_logs_length = startup_logs_length + @output_length WHERE workspace_agents.id = @agent_id
)
INSERT INTO
		workspace_agent_startup_logs (agent_id, created_at, output, level)
	SELECT
		@agent_id :: uuid AS agent_id,
		unnest(@created_at :: timestamptz [ ]) AS created_at,
		unnest(@output :: VARCHAR(1024) [ ]) AS output,
		unnest(@level :: log_level [ ]) AS level
	RETURNING workspace_agent_startup_logs.*;

-- If an agent hasn't connected in the last 7 days, we purge it's logs.
-- Logs can take up a lot of space, so it's important we clean up frequently.
-- name: DeleteOldWorkspaceAgentStartupLogs :exec
DELETE FROM workspace_agent_startup_logs WHERE agent_id IN
	(SELECT id FROM workspace_agents WHERE last_connected_at IS NOT NULL
		AND last_connected_at < NOW() - INTERVAL '7 day');

-- name: GetWorkspaceAgentsInLatestBuildByWorkspaceID :many
SELECT
	workspace_agents.*
FROM
	workspace_agents
JOIN
	workspace_resources ON workspace_agents.resource_id = workspace_resources.id
JOIN
	workspace_builds ON workspace_resources.job_id = workspace_builds.job_id
WHERE
	workspace_builds.workspace_id = @workspace_id :: uuid AND
	workspace_builds.build_number = (
    	SELECT
			MAX(build_number)
    	FROM
			workspace_builds AS wb
    	WHERE
			wb.workspace_id = @workspace_id :: uuid
	);
