-- name: InsertExitNode :one
INSERT INTO
	exit_nodes (
		id,
		organization_id,
		name,
		display_name,
		token_hashed_secret,
		created_at,
		updated_at,
		deleted
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, false) RETURNING *;

-- name: GetExitNodeByID :one
SELECT
	*
FROM
	exit_nodes
WHERE
	id = $1
LIMIT
	1;

-- name: GetExitNodeByOrgAndName :one
SELECT
	*
FROM
	exit_nodes
WHERE
	organization_id = @organization_id
	AND lower(name) = lower(@name)
	AND deleted = false
LIMIT
	1;

-- name: GetExitNodesByOrganization :many
SELECT
	*
FROM
	exit_nodes
WHERE
	organization_id = @organization_id
	AND deleted = false
ORDER BY
	lower(name) ASC;

-- name: UpdateExitNodeRegistration :one
UPDATE
	exit_nodes
SET
	version = @version :: text,
	last_seen_at = @last_seen_at :: timestamptz,
	wireguard_endpoints = @wireguard_endpoints :: text[],
	updated_at = Now()
WHERE
	id = @id
RETURNING *;

-- name: DeleteExitNodeByID :exec
-- Exit nodes are soft-deleted so that audit and connection logs keep a
-- resolvable reference.
UPDATE
	exit_nodes
SET
	updated_at = Now(),
	deleted = true
WHERE
	id = @id;

-- name: GetWorkspaceAgentIDsByExitNode :many
-- GetWorkspaceAgentIDsByExitNode returns the agent IDs on the latest build of
-- every running workspace whose template routes egress through the exit node.
-- "Running" means the latest build has transition=start and
-- job_status=succeeded, matching the workspace-status definition used by
-- coderd/database/queries/workspaces.sql.
SELECT
	workspace_agents.id
FROM
	workspaces
JOIN
	templates
ON
	templates.id = workspaces.template_id
JOIN (
	-- Latest build per workspace.
	SELECT DISTINCT ON (workspace_id)
		id, workspace_id, job_id, transition
	FROM
		workspace_builds
	ORDER BY
		workspace_id, build_number DESC
) AS latest_builds
ON
	latest_builds.workspace_id = workspaces.id
JOIN
	provisioner_jobs
ON
	provisioner_jobs.id = latest_builds.job_id
JOIN
	workspace_resources
ON
	workspace_resources.job_id = latest_builds.job_id
JOIN
	workspace_agents
ON
	workspace_agents.resource_id = workspace_resources.id
WHERE
	templates.exit_node_id = @exit_node_id :: uuid
	AND templates.deleted = FALSE
	AND workspaces.deleted = FALSE
	AND latest_builds.transition = 'start' :: workspace_transition
	AND provisioner_jobs.job_status = 'succeeded' :: provisioner_job_status
	AND workspace_agents.deleted = FALSE;
