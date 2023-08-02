-- name: UpsertTailnetClient :one
INSERT INTO
	tailnet_clients (
	id,
	coordinator_id,
	agent_id,
	node,
	updated_at
)
VALUES
	($1, $2, $3, $4, now() at time zone 'utc')
ON CONFLICT (id, coordinator_id)
DO UPDATE SET
	id = $1,
	coordinator_id = $2,
	agent_id = $3,
	node = $4,
	updated_at = now() at time zone 'utc'
RETURNING *;

-- name: UpsertTailnetAgent :one
INSERT INTO
	tailnet_agents (
	id,
	coordinator_id,
	node,
	updated_at
)
VALUES
	($1, $2, $3, now() at time zone 'utc')
ON CONFLICT (id, coordinator_id)
DO UPDATE SET
	id = $1,
	coordinator_id = $2,
	node = $3,
	updated_at = now() at time zone 'utc'
RETURNING *;


-- name: DeleteTailnetClient :one
DELETE
FROM tailnet_clients
WHERE id = $1 and coordinator_id = $2
RETURNING id, coordinator_id;

-- name: DeleteTailnetAgent :one
DELETE
FROM tailnet_agents
WHERE id = $1 and coordinator_id = $2
RETURNING id, coordinator_id;

-- name: DeleteCoordinator :exec
DELETE
FROM tailnet_coordinators
WHERE id = $1;

-- name: GetTailnetAgents :many
SELECT *
FROM tailnet_agents
WHERE id = $1;

-- name: GetAllTailnetAgents :many
SELECT *
FROM tailnet_agents;

-- name: GetTailnetClientsForAgent :many
SELECT *
FROM tailnet_clients
WHERE agent_id = $1;

-- name: GetAllTailnetClients :many
SELECT *
FROM tailnet_clients
ORDER BY agent_id;

-- name: UpsertTailnetCoordinator :one
INSERT INTO
	tailnet_coordinators (
	id,
	heartbeat_at
)
VALUES
	($1, now() at time zone 'utc')
ON CONFLICT (id)
DO UPDATE SET
  id = $1,
  heartbeat_at = now() at time zone 'utc'
RETURNING *;

-- name: CleanTailnetCoordinators :exec
DELETE
FROM tailnet_coordinators
WHERE heartbeat_at < now() - INTERVAL '24 HOURS';
