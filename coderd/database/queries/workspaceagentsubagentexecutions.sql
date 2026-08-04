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
	UPDATE workspace_agents AS child
	SET subagent_state_version = child.subagent_state_version + 1
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

-- name: AcquireWorkspaceAgentSubagentExecution :one
-- AcquireWorkspaceAgentSubagentExecution hands a parent agent the credentials it
-- needs to launch one declared subagent execution, and fences every previous
-- launcher of the same declaration in the same statement.
--
-- The caller supplies the execution tuple it believes it owns. The query only
-- proceeds when all of the following hold:
--
--   - the exact (workspace_build_id, declaration_id, parent_agent_id) tuple
--     exists, so a parent cannot acquire another parent's declaration;
--   - the parent is live, top-level, and part of the requested build;
--   - the child is exactly the persisted child_agent_id, live, a direct child of
--     the exact parent, on the parent's resource, and execution isolated;
--   - the workspace's actual latest build, resolved here instead of trusted from
--     the caller or a cached middleware build, is the requested build and has
--     transition 'start', so a stale generation cannot launch;
--   - a status row exists and is not 'stopping', so a shutting-down execution is
--     never restarted.
--
-- The status row is locked FOR UPDATE before it is mutated, so concurrent
-- acquisitions serialize and each receives a distinct, monotonically increasing
-- acquisition_version. restart_count is incremented only when the declaration
-- has already been acquired at least once (acquisition_version > 0), so the
-- first launch is not counted as a restart. status_changed_at only moves when
-- the status actually changes, keeping "how long has it been starting" honest
-- across repeated acquisitions.
--
-- Only the child identity, the child auth token, and the new acquisition version
-- are returned. The parent token and the declaration's configuration fields are
-- deliberately excluded.
WITH requested_execution AS (
	SELECT
		executions.workspace_build_id,
		executions.declaration_id,
		executions.child_agent_id,
		builds.workspace_id
	FROM workspace_agent_subagent_executions AS executions
	JOIN workspace_builds AS builds
		ON builds.id = executions.workspace_build_id
	JOIN workspace_agents AS parent
		ON parent.id = executions.parent_agent_id
	JOIN workspace_resources AS parent_resource
		ON parent_resource.id = parent.resource_id
	WHERE executions.workspace_build_id = @workspace_build_id
		AND executions.declaration_id = @declaration_id
		AND executions.parent_agent_id = @parent_agent_id
		AND parent.parent_id IS NULL
		AND parent.deleted = FALSE
		AND parent_resource.job_id = builds.job_id
), latest_build AS (
	SELECT builds.id, builds.transition
	FROM workspace_builds AS builds
	JOIN requested_execution
		ON requested_execution.workspace_id = builds.workspace_id
	ORDER BY builds.build_number DESC
	LIMIT 1
), current_generation AS (
	SELECT requested_execution.*
	FROM requested_execution
	JOIN latest_build
		ON latest_build.id = requested_execution.workspace_build_id
	WHERE latest_build.transition = 'start'
), acquirable_child AS (
	SELECT
		current_generation.workspace_build_id,
		current_generation.declaration_id,
		child.id AS child_agent_id,
		child.auth_token
	FROM current_generation
	JOIN workspace_agents AS parent
		ON parent.id = @parent_agent_id
	JOIN workspace_agents AS child
		ON child.id = current_generation.child_agent_id
		AND child.parent_id = parent.id
		AND child.resource_id = parent.resource_id
		AND child.deleted = FALSE
		AND child.execution_isolation = TRUE
), locked_status AS MATERIALIZED (
	SELECT
		statuses.workspace_build_id,
		statuses.declaration_id,
		statuses.status,
		statuses.status_changed_at,
		statuses.restart_count,
		statuses.acquisition_version
	FROM workspace_agent_subagent_execution_statuses AS statuses
	JOIN acquirable_child
		USING (workspace_build_id, declaration_id)
	WHERE statuses.status != 'stopping'
	FOR UPDATE OF statuses
), acquired AS (
	UPDATE workspace_agent_subagent_execution_statuses AS statuses
	SET
		acquisition_version = locked_status.acquisition_version + 1,
		status = 'starting',
		updated_at = @now,
		status_changed_at = CASE
			WHEN locked_status.status = 'starting' THEN locked_status.status_changed_at
			ELSE @now
		END,
		last_acquired_at = @now,
		last_error = '',
		restart_count = CASE
			WHEN locked_status.acquisition_version > 0 THEN locked_status.restart_count + 1
			ELSE locked_status.restart_count
		END
	FROM locked_status
	WHERE statuses.workspace_build_id = locked_status.workspace_build_id
		AND statuses.declaration_id = locked_status.declaration_id
	RETURNING statuses.acquisition_version
)
SELECT
	acquirable_child.child_agent_id,
	acquirable_child.auth_token,
	acquired.acquisition_version
FROM acquired
JOIN acquirable_child ON TRUE;

-- name: GetWorkspaceAgentSubagentExecutionsByParentAgentID :many
SELECT *
FROM workspace_agent_subagent_executions
WHERE parent_agent_id = $1
ORDER BY created_at, declaration_id;

-- name: GetWorkspaceAgentSubagentExecutionDeclarationsByParentAgentID :many
-- GetWorkspaceAgentSubagentExecutionDeclarationsByParentAgentID returns the
-- non-secret declaration fields a parent agent needs to render its execution
-- manifest. The child auth token is deliberately excluded. The child agent is
-- resolved with a LEFT JOIN restricted to the exact parent, so a declaration
-- whose child is missing, reparented, deleted, or no longer execution isolated
-- yields an empty child name instead of being silently omitted or retargeted.
-- Callers must treat an empty name as a corrupted manifest and fail closed.
SELECT
	executions.workspace_build_id,
	executions.declaration_id,
	COALESCE(child.name, '') AS child_agent_name,
	executions.driver,
	executions.driver_protocol,
	executions.shared_host_path,
	executions.shared_child_path,
	executions.startup_timeout_seconds,
	executions.restart_policy
FROM workspace_agent_subagent_executions AS executions
LEFT JOIN workspace_agents AS child
	ON child.id = executions.child_agent_id
	AND child.parent_id = executions.parent_agent_id
	AND child.deleted = FALSE
	AND child.execution_isolation = TRUE
WHERE executions.parent_agent_id = $1
ORDER BY executions.created_at, executions.declaration_id;

-- name: GetWorkspaceAgentSubagentExecutionStatus :one
SELECT workspace_agent_subagent_execution_statuses.*
FROM workspace_agent_subagent_execution_statuses
JOIN workspace_agent_subagent_executions
	USING (workspace_build_id, declaration_id)
WHERE workspace_agent_subagent_execution_statuses.workspace_build_id = @workspace_build_id
	AND workspace_agent_subagent_execution_statuses.declaration_id = @declaration_id
	AND workspace_agent_subagent_executions.parent_agent_id = @parent_agent_id;
