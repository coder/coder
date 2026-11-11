-- name: InsertWorkloadIdentity :one
INSERT INTO workload_identities (
	id,
	created_at,
	organization_id,
	sponsor_id,
	workspace_id,
	name,
	scopes
)
VALUES (
	@id,
	@created_at,
	@organization_id,
	@sponsor_id,
	@workspace_id,
	@name,
	@scopes
)
RETURNING *;

-- name: GetWorkloadIdentityByWorkspaceAgentID :one
SELECT
	*
FROM
	workload_identities
WHERE
	workspace_agent_id = @workspace_agent_id
	AND revoked_at IS NULL;

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

-- BindWorkloadIdentity records the daemon that runs the workload and what the
-- workload proves to act as the identity. An identity is bound at most once.
-- name: BindWorkloadIdentity :one
UPDATE
	workload_identities
SET
	workspace_agent_id = @workspace_agent_id,
	attestation_subject = @attestation_subject,
	bound_at = @bound_at
WHERE
	id = @id
	AND workspace_agent_id IS NULL
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

-- name: RevokeWorkloadIdentityByWorkspaceAgentID :exec
UPDATE
	workload_identities
SET
	revoked_at = @revoked_at
WHERE
	workspace_agent_id = @workspace_agent_id
	AND revoked_at IS NULL;
