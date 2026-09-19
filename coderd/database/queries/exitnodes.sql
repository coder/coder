-- name: InsertExitNode :one
INSERT INTO exit_nodes (id, organization_id, name, display_name, token_hashed_secret, created_at, updated_at, deleted)
VALUES ($1, $2, $3, $4, $5, $6, $7, false)
RETURNING *;

-- name: GetExitNodeByID :one
SELECT * FROM exit_nodes WHERE id = $1 LIMIT 1;

-- name: GetExitNodeByOrgAndName :one
SELECT * FROM exit_nodes
WHERE organization_id = @organization_id AND lower(name) = lower(@name) AND deleted = false
LIMIT 1;

-- name: GetExitNodesByOrganization :many
SELECT * FROM exit_nodes
WHERE organization_id = @organization_id AND deleted = false
ORDER BY lower(name) ASC;

-- name: GetExitNodeReplicaByID :one
SELECT * FROM exit_node_replicas WHERE id = @id;

-- name: UpsertExitNodeReplica :one
INSERT INTO exit_node_replicas (
	id,
	exit_node_id,
	hostname,
	version,
	wireguard_endpoints,
	policy_hash,
	created_at,
	started_at,
	updated_at
) VALUES (
	@id,
	@exit_node_id,
	@hostname,
	@version,
	@wireguard_endpoints::text[],
	@policy_hash,
	@now,
	@now,
	@now
)
ON CONFLICT (id) DO UPDATE SET
	hostname = EXCLUDED.hostname,
	version = EXCLUDED.version,
	wireguard_endpoints = EXCLUDED.wireguard_endpoints,
	policy_hash = EXCLUDED.policy_hash,
	updated_at = EXCLUDED.updated_at
WHERE
	exit_node_replicas.exit_node_id = EXCLUDED.exit_node_id
	AND exit_node_replicas.stopped_at IS NULL
RETURNING *;

-- name: StopExitNodeReplica :exec
UPDATE exit_node_replicas
SET stopped_at = @stopped_at::timestamptz
WHERE id = @id;

-- name: GetLiveExitNodeReplicas :many
SELECT *
FROM exit_node_replicas
WHERE
	exit_node_id = ANY(@exit_node_ids::uuid[])
	AND stopped_at IS NULL
	AND updated_at > @updated_after
ORDER BY exit_node_id, started_at, id;

-- name: GetExitNodeReplicasByExitNode :many
SELECT *
FROM exit_node_replicas
WHERE exit_node_id = @exit_node_id
ORDER BY started_at, id;

-- name: GetTemplateExitNodeReplicas :many
SELECT
	ten.exit_node_id,
	ten.position,
	r.id AS replica_id,
	r.wireguard_endpoints
FROM template_exit_nodes AS ten
JOIN exit_nodes AS en ON en.id = ten.exit_node_id AND en.deleted = false
LEFT JOIN exit_node_replicas AS r ON
	r.exit_node_id = en.id
	AND r.stopped_at IS NULL
	AND r.updated_at > @updated_after
WHERE ten.template_id = @template_id
ORDER BY ten.position, r.started_at, r.id;

-- name: DeleteStaleExitNodeReplicas :exec
DELETE FROM exit_node_replicas WHERE updated_at < @updated_before;

-- name: DeleteExitNodeByID :exec
-- Exit nodes are soft-deleted so that audit and connection logs keep a
-- resolvable reference.
UPDATE exit_nodes SET updated_at = Now(), deleted = true WHERE id = @id;

-- name: GetTemplateExitNodes :many
SELECT exit_nodes.*
FROM template_exit_nodes
JOIN exit_nodes ON exit_nodes.id = template_exit_nodes.exit_node_id
WHERE template_exit_nodes.template_id = @template_id
ORDER BY template_exit_nodes.position;

-- name: DeleteTemplateExitNodes :exec
DELETE FROM template_exit_nodes
WHERE template_id = @template_id;

-- name: InsertTemplateExitNodes :exec
INSERT INTO template_exit_nodes (template_id, exit_node_id, position)
SELECT @template_id, exit_node_id, ordinality - 1
FROM unnest(@exit_node_ids::uuid[]) WITH ORDINALITY AS nodes(exit_node_id, ordinality);

-- name: GetWorkspaceAgentIDsByExitNode :many
-- GetWorkspaceAgentIDsByExitNode returns the agent IDs on the latest build of
-- every running workspace whose template routes egress through the exit node.
-- "Running" means the latest build has transition=start and
-- job_status=succeeded, matching the workspace-status definition used by
-- coderd/database/queries/workspaces.sql.
SELECT workspace_agents.id
FROM workspaces
JOIN templates ON templates.id = workspaces.template_id
JOIN template_exit_nodes ON template_exit_nodes.template_id = templates.id
JOIN (
	-- Latest build per workspace.
	SELECT DISTINCT ON (workspace_id) id, workspace_id, job_id, transition
	FROM workspace_builds
	ORDER BY workspace_id, build_number DESC
) AS latest_builds ON latest_builds.workspace_id = workspaces.id
JOIN provisioner_jobs ON provisioner_jobs.id = latest_builds.job_id
JOIN workspace_resources ON workspace_resources.job_id = latest_builds.job_id
JOIN workspace_agents ON workspace_agents.resource_id = workspace_resources.id
WHERE
	template_exit_nodes.exit_node_id = @exit_node_id :: uuid
	AND templates.deleted = FALSE
	AND workspaces.deleted = FALSE
	AND latest_builds.transition = 'start' :: workspace_transition
	AND provisioner_jobs.job_status = 'succeeded' :: provisioner_job_status
	AND workspace_agents.deleted = FALSE;
