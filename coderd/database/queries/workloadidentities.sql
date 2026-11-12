-- name: InsertWorkloadIdentity :one
INSERT INTO workload_identities (
	id,
	created_at,
	organization_id,
	sponsor_id,
	workspace_id,
	workspace_agent_id,
	user_id,
	name,
	command,
	scopes,
	interactive,
	remove_workspace
)
VALUES (
	@id,
	@created_at,
	@organization_id,
	@sponsor_id,
	@workspace_id,
	@workspace_agent_id,
	@user_id,
	@name,
	@command,
	@scopes,
	@interactive,
	@remove_workspace
)
RETURNING *;

-- GetWorkloadIdentityByID also returns finished identities. Callers that
-- authorize with the identity must check finished_at.
-- name: GetWorkloadIdentityByID :one
SELECT
	*
FROM
	workload_identities
WHERE
	id = @id;

-- GetWorkloadIdentityByUserID returns the identity a service account
-- is, whether or not its workload is over.
-- name: GetWorkloadIdentityByUserID :one
SELECT
	*
FROM
	workload_identities
WHERE
	user_id = @user_id;

-- GetWorkloadIdentitiesByWorkspaceID returns the live workloads of a
-- workspace.
-- name: GetWorkloadIdentitiesByWorkspaceID :many
SELECT
	*
FROM
	workload_identities
WHERE
	workspace_id = @workspace_id
	AND finished_at IS NULL
ORDER BY
	created_at ASC;

-- GetWorkloadIdentitiesBySponsorID returns a sponsor's live workloads in
-- every workspace.
-- name: GetWorkloadIdentitiesBySponsorID :many
SELECT
	*
FROM
	workload_identities
WHERE
	sponsor_id = @sponsor_id
	AND finished_at IS NULL
ORDER BY
	created_at ASC;

-- GetFinishedWorkloadIdentitiesBySponsorID returns a sponsor's most recently
-- finished workloads in every workspace, newest first. The workspaces may
-- be gone.
-- name: GetFinishedWorkloadIdentitiesBySponsorID :many
SELECT
	*
FROM
	workload_identities
WHERE
	sponsor_id = @sponsor_id
	AND finished_at IS NOT NULL
ORDER BY
	finished_at DESC
LIMIT
	@limit_opt :: integer;

-- GetPendingWorkloadIdentitiesByWorkspaceAgentID returns the workloads a
-- daemon has been given and has not started yet. A workload that was not
-- started in time is no longer offered.
-- name: GetPendingWorkloadIdentitiesByWorkspaceAgentID :many
SELECT
	*
FROM
	workload_identities
WHERE
	workspace_agent_id = @workspace_agent_id
	AND started_at IS NULL
	AND finished_at IS NULL
	AND created_at > @created_after
ORDER BY
	created_at ASC;

-- GetFinishedWorkloadIdentitiesByWorkspaceID returns the most recently
-- finished workloads of a workspace, newest first.
-- name: GetFinishedWorkloadIdentitiesByWorkspaceID :many
SELECT
	*
FROM
	workload_identities
WHERE
	workspace_id = @workspace_id
	AND finished_at IS NOT NULL
ORDER BY
	finished_at DESC
LIMIT
	@limit_opt :: integer;

-- UpdateWorkloadIdentityWorkspaceAgent gives a workload to the daemon of
-- the workspace that was built for it.
-- name: UpdateWorkloadIdentityWorkspaceAgent :one
UPDATE
	workload_identities
SET
	workspace_agent_id = @workspace_agent_id
WHERE
	id = @id
	AND finished_at IS NULL
RETURNING *;

-- UpdateWorkloadIdentitySandbox records where the daemon started the
-- workload.
-- name: UpdateWorkloadIdentitySandbox :one
UPDATE
	workload_identities
SET
	runtime = @runtime,
	sandbox_id = @sandbox_id,
	started_at = @started_at
WHERE
	id = @id
	AND finished_at IS NULL
RETURNING *;

-- FinishWorkloadIdentity ends a workload. From then on the identity
-- authorizes nothing. It returns no row when the workload was already over,
-- so that an ending is recorded once.
-- name: FinishWorkloadIdentity :one
UPDATE
	workload_identities
SET
	finished_at = @finished_at,
	end_reason = @end_reason,
	exit_code = @exit_code,
	output_tail = @output_tail
WHERE
	id = @id
	AND finished_at IS NULL
RETURNING *;

-- UpdateWorkloadIdentityOutcome adds what the daemon knows about how a
-- workload ended to one that was finished before the daemon reported in,
-- for example a workload a user removed.
-- name: UpdateWorkloadIdentityOutcome :exec
UPDATE
	workload_identities
SET
	exit_code = COALESCE(exit_code, @exit_code),
	output_tail = CASE WHEN output_tail = '' THEN @output_tail ELSE output_tail END
WHERE
	id = @id
	AND finished_at IS NOT NULL;
