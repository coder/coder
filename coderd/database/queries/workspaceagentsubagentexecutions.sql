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

-- name: GetWorkspaceAgentSubagentExecutionForAcquisition :one
-- GetWorkspaceAgentSubagentExecutionForAcquisition resolves the immutable
-- identity behind an acquisition request: the workspace that owns the requested
-- build, and the child agent the declaration was created with. The exact
-- (workspace_build_id, declaration_id, parent_agent_id) tuple must exist, so a
-- parent cannot acquire another parent's declaration. Both returned values are
-- immutable for the life of the rows, so reading them before the publication
-- lock is taken cannot yield a stale answer. Every mutable fact (latest build,
-- parent liveness, child liveness, status) is read by a later statement, after
-- the lock, so it observes a fresh READ COMMITTED snapshot.
SELECT
	builds.workspace_id,
	executions.child_agent_id
FROM workspace_agent_subagent_executions AS executions
JOIN workspace_builds AS builds
	ON builds.id = executions.workspace_build_id
WHERE executions.workspace_build_id = @workspace_build_id
	AND executions.declaration_id = @declaration_id
	AND executions.parent_agent_id = @parent_agent_id;

-- name: AcquireWorkspaceBuildPublicationLock :exec
-- AcquireWorkspaceBuildPublicationLock takes the transaction-scoped advisory
-- lock that workspace build inserts also take through the
-- serialize_workspace_build_publication trigger. Holding it means no new build
-- for this workspace can become visible while the acquisition validates the
-- workspace's latest generation and mutates the execution status.
--
-- This must be called from within a transaction. The lock is released when the
-- transaction ends.
SELECT acquire_workspace_build_publication_lock(@workspace_id);

-- name: LockWorkspaceAgentSubagentExecutionStatusForAcquisition :one
-- LockWorkspaceAgentSubagentExecutionStatusForAcquisition locks the exact status
-- row that the acquisition will mutate, and rejects a declaration that is
-- already shutting down so a stopping execution is never restarted. Concurrent
-- acquisitions of the same declaration serialize here, which is what makes
-- acquisition_version strictly increasing.
SELECT
	statuses.status,
	statuses.status_changed_at,
	statuses.restart_count,
	statuses.acquisition_version
FROM workspace_agent_subagent_execution_statuses AS statuses
WHERE statuses.workspace_build_id = @workspace_build_id
	AND statuses.declaration_id = @declaration_id
	AND statuses.status != 'stopping'
FOR UPDATE;

-- name: GetLatestWorkspaceBuildGeneration :one
-- GetLatestWorkspaceBuildGeneration resolves the workspace's actual latest build
-- instead of trusting a caller-supplied or middleware-cached generation. It runs
-- as its own statement after the publication lock is held, so its snapshot
-- cannot predate a build that committed while the caller was starting up.
SELECT
	builds.id,
	builds.transition
FROM workspace_builds AS builds
WHERE builds.workspace_id = @workspace_id
ORDER BY builds.build_number DESC
LIMIT 1;

-- name: GetWorkspaceAgentSubagentExecutionAcquisitionParent :one
-- GetWorkspaceAgentSubagentExecutionAcquisitionParent re-reads the requesting
-- parent under the publication lock and requires it to still be live, top-level,
-- and part of the requested build through its resource's provisioner job. The
-- parent's resource is returned so the child can be matched against the exact
-- resource rather than any resource in the workspace.
SELECT
	parent.id,
	parent.resource_id
FROM workspace_agents AS parent
JOIN workspace_resources AS parent_resource
	ON parent_resource.id = parent.resource_id
JOIN workspace_builds AS builds
	ON builds.job_id = parent_resource.job_id
WHERE parent.id = @parent_agent_id
	AND parent.parent_id IS NULL
	AND parent.deleted = FALSE
	AND builds.id = @workspace_build_id;

-- name: LockWorkspaceAgentSubagentExecutionChildForAcquisition :one
-- LockWorkspaceAgentSubagentExecutionChildForAcquisition locks the exact child
-- the declaration was created with and requires it to still be a live, execution
-- isolated, direct child on the parent's resource.
--
-- The lock is FOR SHARE rather than FOR KEY SHARE on purpose: an ordinary
-- soft-delete only touches non-key columns, so FOR KEY SHARE would not conflict
-- with it. FOR SHARE conflicts with any concurrent UPDATE of this row and forces
-- an EvalPlanQual recheck of the predicates above, so a child soft-deleted by a
-- committing rebuild cannot be handed out.
SELECT
	child.id,
	child.auth_token
FROM workspace_agents AS child
WHERE child.id = @child_agent_id
	AND child.parent_id = @parent_agent_id
	AND child.resource_id = @resource_id
	AND child.deleted = FALSE
	AND child.execution_isolation = TRUE
FOR SHARE;

-- name: MarkWorkspaceAgentSubagentExecutionAcquired :one
-- MarkWorkspaceAgentSubagentExecutionAcquired records the acquisition on the
-- status row that this transaction already locked FOR UPDATE, and fences every
-- previous launcher of the same declaration by bumping acquisition_version.
--
-- restart_count is only incremented when the declaration has already been
-- acquired at least once (acquisition_version > 0), so the first launch is not
-- counted as a restart. status_changed_at only moves when the status actually
-- changes, keeping "how long has it been starting" honest across repeated
-- acquisitions.
--
-- Only the child identity, the child auth token, and the new acquisition version
-- are returned. The parent token and the declaration's configuration fields are
-- deliberately excluded. The child row was already locked FOR SHARE by this
-- transaction, so joining it here cannot observe a newer child.
UPDATE workspace_agent_subagent_execution_statuses AS statuses
SET
	acquisition_version = statuses.acquisition_version + 1,
	status = 'starting',
	updated_at = @now,
	status_changed_at = CASE
		WHEN statuses.status = 'starting' THEN statuses.status_changed_at
		ELSE @now
	END,
	last_acquired_at = @now,
	last_error = '',
	restart_count = CASE
		WHEN statuses.acquisition_version > 0 THEN statuses.restart_count + 1
		ELSE statuses.restart_count
	END
FROM workspace_agents AS child
WHERE statuses.workspace_build_id = @workspace_build_id
	AND statuses.declaration_id = @declaration_id
	AND child.id = @child_agent_id
RETURNING
	child.id AS child_agent_id,
	child.auth_token,
	statuses.acquisition_version;

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
