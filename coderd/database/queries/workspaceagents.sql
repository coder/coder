-- name: GetWorkspaceAgentByID :one
SELECT
	*
FROM
	workspace_agents
WHERE
	id = $1
	-- Filter out deleted sub agents.
	AND deleted = FALSE;

-- name: GetWorkspaceAgentByInstanceID :one
SELECT
	*
FROM
	workspace_agents
WHERE
	auth_instance_id = @auth_instance_id :: TEXT
	-- Filter out deleted sub agents.
	AND deleted = FALSE
ORDER BY
	created_at DESC;

-- name: GetWorkspaceAgentsByResourceIDs :many
SELECT
	*
FROM
	workspace_agents
WHERE
	resource_id = ANY(@ids :: uuid [ ])
	-- Filter out deleted sub agents.
	AND deleted = FALSE;

-- name: GetWorkspaceAgentsCreatedAfter :many
SELECT * FROM workspace_agents
WHERE
	created_at > $1
	-- Filter out deleted sub agents.
	AND deleted = FALSE;

-- name: InsertWorkspaceAgent :one
INSERT INTO
	workspace_agents (
		id,
		parent_id,
		created_at,
		updated_at,
		name,
		resource_id,
		auth_token,
		auth_instance_id,
		architecture,
		environment_variables,
		operating_system,
		directory,
		instance_metadata,
		resource_metadata,
		connection_timeout_seconds,
		troubleshooting_url,
		motd_file,
		display_apps,
		display_order,
		api_key_scope
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20) RETURNING *;

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
	subsystems = $4,
	api_version = $5
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
		interval,
		display_order
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7);

-- name: UpdateWorkspaceAgentMetadata :exec
WITH metadata AS (
	SELECT
		unnest(sqlc.arg('key')::text[]) AS key,
		unnest(sqlc.arg('value')::text[]) AS value,
		unnest(sqlc.arg('error')::text[]) AS error,
		unnest(sqlc.arg('collected_at')::timestamptz[]) AS collected_at
)
UPDATE
	workspace_agent_metadata wam
SET
	value = m.value,
	error = m.error,
	collected_at = m.collected_at
FROM
	metadata m
WHERE
	wam.workspace_agent_id = $1
	AND wam.key = m.key;

-- name: GetWorkspaceAgentMetadata :many
SELECT
	*
FROM
	workspace_agent_metadata
WHERE
	workspace_agent_id = $1
	AND CASE WHEN COALESCE(array_length(sqlc.arg('keys')::text[], 1), 0) > 0 THEN key = ANY(sqlc.arg('keys')::text[]) ELSE TRUE END;

-- name: UpdateWorkspaceAgentLogOverflowByID :exec
UPDATE
	workspace_agents
SET
	logs_overflowed = $2
WHERE
	id = $1;

-- name: GetWorkspaceAgentLogsAfter :many
SELECT
	*
FROM
	workspace_agent_logs
WHERE
	agent_id = $1
	AND (
		id > @created_after
	) ORDER BY id ASC;

-- name: InsertWorkspaceAgentLogs :many
WITH new_length AS (
	UPDATE workspace_agents SET
	logs_length = logs_length + @output_length WHERE workspace_agents.id = @agent_id
)
INSERT INTO
		workspace_agent_logs (agent_id, created_at, output, level, log_source_id)
	SELECT
		@agent_id :: uuid AS agent_id,
		@created_at :: timestamptz AS created_at,
		unnest(@output :: VARCHAR(1024) [ ]) AS output,
		unnest(@level :: log_level [ ]) AS level,
		@log_source_id :: uuid AS log_source_id
	RETURNING workspace_agent_logs.*;

-- name: InsertWorkspaceAgentLogSources :many
INSERT INTO
		workspace_agent_log_sources (workspace_agent_id, created_at, id, display_name, icon)
	SELECT
		@workspace_agent_id :: uuid AS workspace_agent_id,
		@created_at :: timestamptz AS created_at,
		unnest(@id :: uuid [ ]) AS id,
		unnest(@display_name :: VARCHAR(127) [ ]) AS display_name,
		unnest(@icon :: text [ ]) AS icon
	RETURNING workspace_agent_log_sources.*;

-- name: GetWorkspaceAgentLogSourcesByAgentIDs :many
SELECT * FROM workspace_agent_log_sources WHERE workspace_agent_id = ANY(@ids :: uuid [ ]);

-- If an agent hasn't connected within the retention period, we purge its logs.
-- Exception: if the logs are related to the latest build, we keep those around.
-- Logs can take up a lot of space, so it's important we clean up frequently.
-- name: DeleteOldWorkspaceAgentLogs :execrows
WITH
	latest_builds AS (
		SELECT
			workspace_id, max(build_number) AS max_build_number
		FROM
			workspace_builds
		GROUP BY
			workspace_id
	),
	old_agents AS (
		SELECT
			wa.id
		FROM
			workspace_agents AS wa
		JOIN
			workspace_resources AS wr
		ON
			wa.resource_id = wr.id
		JOIN
			workspace_builds AS wb
		ON
			wb.job_id = wr.job_id
		LEFT JOIN
			latest_builds
		ON
			latest_builds.workspace_id = wb.workspace_id
		AND
			latest_builds.max_build_number = wb.build_number
		WHERE
			-- Filter out the latest builds for each workspace.
			latest_builds.workspace_id IS NULL
		AND CASE
			-- If the last time the agent connected was before @threshold
			WHEN wa.last_connected_at IS NOT NULL THEN
				 wa.last_connected_at < @threshold :: timestamptz
			-- The agent never connected, and was created before @threshold
			ELSE wa.created_at < @threshold :: timestamptz
		END
	)
DELETE FROM workspace_agent_logs WHERE agent_id IN (SELECT id FROM old_agents);

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
	)
	-- Filter out deleted sub agents.
	AND workspace_agents.deleted = FALSE;

-- name: GetWorkspaceAgentsByWorkspaceAndBuildNumber :many
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
	workspace_builds.build_number = @build_number :: int
	-- Filter out deleted sub agents.
	AND workspace_agents.deleted = FALSE;

-- name: GetWorkspaceAgentAndLatestBuildByAuthToken :one
SELECT
	sqlc.embed(workspaces),
	sqlc.embed(workspace_agents),
	sqlc.embed(workspace_build_with_user),
	tasks.id AS task_id
FROM
	workspace_agents
JOIN
	workspace_resources
ON
	workspace_agents.resource_id = workspace_resources.id
JOIN
	workspace_build_with_user
ON
	workspace_resources.job_id = workspace_build_with_user.job_id
JOIN
	workspaces
ON
	workspace_build_with_user.workspace_id = workspaces.id
LEFT JOIN
	tasks
ON
	tasks.workspace_id = workspaces.id
WHERE
	-- This should only match 1 agent, so 1 returned row or 0.
	workspace_agents.auth_token = @auth_token::uuid
	AND workspaces.deleted = FALSE
	-- Filter out deleted sub agents.
	AND workspace_agents.deleted = FALSE
	-- Filter out builds that are not the latest.
	AND workspace_build_with_user.build_number = (
		-- Select from workspace_builds as it's one less join compared
		-- to workspace_build_with_user.
		SELECT
			MAX(build_number)
		FROM
			workspace_builds
		WHERE
			workspace_id = workspace_build_with_user.workspace_id
	)
;

-- name: InsertWorkspaceAgentScriptTimings :one
INSERT INTO
    workspace_agent_script_timings (
        script_id,
        started_at,
        ended_at,
        exit_code,
        stage,
        status
    )
VALUES
    ($1, $2, $3, $4, $5, $6)
RETURNING workspace_agent_script_timings.*;

-- name: GetWorkspaceAgentScriptTimingsByBuildID :many
SELECT
	DISTINCT ON (workspace_agent_script_timings.script_id) workspace_agent_script_timings.*,
	workspace_agent_scripts.display_name,
	workspace_agents.id as workspace_agent_id,
	workspace_agents.name as workspace_agent_name
FROM workspace_agent_script_timings
INNER JOIN workspace_agent_scripts ON workspace_agent_scripts.id = workspace_agent_script_timings.script_id
INNER JOIN workspace_agents ON workspace_agents.id = workspace_agent_scripts.workspace_agent_id
INNER JOIN workspace_resources ON workspace_resources.id = workspace_agents.resource_id
INNER JOIN workspace_builds ON workspace_builds.job_id = workspace_resources.job_id
WHERE workspace_builds.id = $1
ORDER BY workspace_agent_script_timings.script_id, workspace_agent_script_timings.started_at;

-- name: GetWorkspaceAgentsByParentID :many
SELECT
	*
FROM
	workspace_agents
WHERE
	parent_id = @parent_id::uuid
	AND deleted = FALSE;

-- name: DeleteWorkspaceSubAgentByID :exec
UPDATE
	workspace_agents
SET
	deleted = TRUE
WHERE
	id = $1
	AND parent_id IS NOT NULL
	AND deleted = FALSE;

-- name: GetWorkspaceAgentsForMetrics :many
SELECT
    w.id as workspace_id,
    w.name as workspace_name,
    u.username as owner_username,
    t.name as template_name,
    tv.name as template_version_name,
    sqlc.embed(workspace_agents)
FROM workspaces w
JOIN users u ON w.owner_id = u.id
JOIN templates t ON w.template_id = t.id
JOIN workspace_builds wb ON w.id = wb.workspace_id
LEFT JOIN template_versions tv ON wb.template_version_id = tv.id
JOIN workspace_resources wr ON wb.job_id = wr.job_id
JOIN workspace_agents ON wr.id = workspace_agents.resource_id
WHERE w.deleted = false
AND wb.build_number = (
    SELECT MAX(wb2.build_number)
    FROM workspace_builds wb2
    WHERE wb2.workspace_id = w.id
)
AND workspace_agents.deleted = FALSE;

-- name: GetWorkspaceAgentByIDWithWorkspace :one
SELECT
	sqlc.embed(workspace_agents),
	sqlc.embed(workspaces_expanded)
FROM
	workspace_agents
JOIN
	workspace_resources ON workspace_agents.resource_id = workspace_resources.id
JOIN
	provisioner_jobs ON workspace_resources.job_id = provisioner_jobs.id
JOIN
	workspace_builds ON provisioner_jobs.id = workspace_builds.job_id
JOIN
	workspaces_expanded ON workspace_builds.workspace_id = workspaces_expanded.id
WHERE
	workspace_agents.id = @id
	AND provisioner_jobs.type = 'workspace_build'::provisioner_job_type
	AND workspaces_expanded.deleted = FALSE
LIMIT 1;
