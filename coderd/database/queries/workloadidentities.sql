-- name: InsertWorkloadIdentity :one
INSERT INTO workload_identities (
	id,
	created_at,
	organization_id,
	sponsor_id,
	workspace_id,
	workspace_agent_id,
	name,
	command,
	scopes
)
VALUES (
	@id,
	@created_at,
	@organization_id,
	@sponsor_id,
	@workspace_id,
	@workspace_agent_id,
	@name,
	@command,
	@scopes
)
RETURNING *;

-- GetWorkloadIdentityByID also returns revoked identities. Callers that
-- authorize with the identity must check revoked_at.
-- name: GetWorkloadIdentityByID :one
SELECT
	*
FROM
	workload_identities
WHERE
	id = @id;

-- GetWorkloadIdentitiesByWorkspaceID does not return revoked identities.
-- name: GetWorkloadIdentitiesByWorkspaceID :many
SELECT
	*
FROM
	workload_identities
WHERE
	workspace_id = @workspace_id
	AND revoked_at IS NULL
ORDER BY
	created_at ASC;

-- UpdateWorkloadIdentitySandbox records the sandbox the daemon started for
-- the workload.
-- name: UpdateWorkloadIdentitySandbox :one
UPDATE
	workload_identities
SET
	runtime = @runtime,
	sandbox_id = @sandbox_id,
	started_at = @started_at
WHERE
	id = @id
	AND revoked_at IS NULL
RETURNING *;

-- name: RevokeWorkloadIdentityByID :exec
UPDATE
	workload_identities
SET
	revoked_at = @revoked_at
WHERE
	id = @id
	AND revoked_at IS NULL;
