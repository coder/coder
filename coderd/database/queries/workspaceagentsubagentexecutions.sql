-- name: InsertWorkspaceAgentSubagentExecution :one
WITH locked_parent AS MATERIALIZED (
	SELECT parent.id, parent.resource_id
	FROM workspace_agents AS parent
	JOIN workspace_resources
		ON workspace_resources.id = parent.resource_id
	JOIN workspace_builds
		ON workspace_builds.job_id = workspace_resources.job_id
	WHERE parent.id = @parent_agent_id
		AND parent.parent_id IS NULL
		AND parent.deleted = FALSE
		AND workspace_builds.id = @workspace_build_id
	FOR KEY SHARE OF parent
), updated_child AS MATERIALIZED (
	-- Updating updated_at is a deliberate version bump used by the legacy
	-- delete's optimistic guard.
	UPDATE workspace_agents AS child
	SET updated_at = GREATEST(
		clock_timestamp(),
		child.updated_at + INTERVAL '1 microsecond'
	)
	FROM locked_parent
	WHERE child.id = @child_agent_id
		AND child.parent_id = locked_parent.id
		AND child.resource_id = locked_parent.resource_id
		AND child.deleted = FALSE
		AND child.execution_isolation = TRUE
	RETURNING child.id
), inserted_execution AS (
	INSERT INTO workspace_agent_subagent_executions (
		workspace_build_id,
		declaration_id,
		parent_agent_id,
		child_agent_id,
		driver,
		driver_protocol,
		shared_host_path,
		shared_child_path,
		startup_timeout_seconds,
		restart_policy
	)
	SELECT
		@workspace_build_id,
		@declaration_id,
		locked_parent.id,
		updated_child.id,
		@driver,
		@driver_protocol,
		@shared_host_path,
		@shared_child_path,
		@startup_timeout_seconds,
		@restart_policy
	FROM locked_parent
	JOIN updated_child ON TRUE
	RETURNING *
), inserted_status AS (
	INSERT INTO workspace_agent_subagent_execution_statuses (
		workspace_build_id,
		declaration_id
	)
	SELECT
		workspace_build_id,
		declaration_id
	FROM inserted_execution
	RETURNING workspace_build_id, declaration_id
)
SELECT inserted_execution.*
FROM inserted_execution
JOIN inserted_status USING (workspace_build_id, declaration_id);

-- name: GetWorkspaceAgentSubagentExecutionsByParentAgentID :many
SELECT *
FROM workspace_agent_subagent_executions
WHERE parent_agent_id = $1
ORDER BY created_at, declaration_id;

-- name: GetWorkspaceAgentSubagentExecutionStatus :one
SELECT workspace_agent_subagent_execution_statuses.*
FROM workspace_agent_subagent_execution_statuses
JOIN workspace_agent_subagent_executions
	USING (workspace_build_id, declaration_id)
WHERE workspace_agent_subagent_execution_statuses.workspace_build_id = @workspace_build_id
	AND workspace_agent_subagent_execution_statuses.declaration_id = @declaration_id
	AND workspace_agent_subagent_executions.parent_agent_id = @parent_agent_id;
