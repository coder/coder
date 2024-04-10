-- name: UpsertTailnetClient :one
INSERT INTO
	tailnet_clients (
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

-- name: UpsertTailnetClientSubscription :exec
INSERT INTO
	tailnet_client_subscriptions (
	client_id,
	coordinator_id,
	agent_id,
	updated_at
)
VALUES
	($1, $2, $3, now() at time zone 'utc')
ON CONFLICT (client_id, coordinator_id, agent_id)
DO UPDATE SET
	client_id = $1,
	coordinator_id = $2,
	agent_id = $3,
	updated_at = now() at time zone 'utc';

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

-- name: DeleteTailnetClientSubscription :exec
DELETE
FROM tailnet_client_subscriptions
WHERE client_id = $1 and agent_id = $2 and coordinator_id = $3;

-- name: DeleteAllTailnetClientSubscriptions :exec
DELETE
FROM tailnet_client_subscriptions
WHERE client_id = $1 and coordinator_id = $2;

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
WHERE id IN (
	SELECT tailnet_client_subscriptions.client_id
	FROM tailnet_client_subscriptions
	WHERE tailnet_client_subscriptions.agent_id = $1
);

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

-- name: CleanTailnetLostPeers :exec
DELETE
FROM tailnet_peers
WHERE updated_at < now() - INTERVAL '24 HOURS' AND status = 'lost'::tailnet_status;

-- name: CleanTailnetTunnels :exec
DELETE FROM tailnet_tunnels
WHERE updated_at < now() - INTERVAL '24 HOURS' AND
      NOT EXISTS (
        SELECT 1 FROM tailnet_peers
        WHERE id = tailnet_tunnels.src_id AND coordinator_id = tailnet_tunnels.coordinator_id
      );

-- name: UpsertTailnetPeer :one
INSERT INTO
	tailnet_peers (
	id,
	coordinator_id,
	node,
	status,
	updated_at
)
VALUES
	($1, $2, $3, $4, now() at time zone 'utc')
ON CONFLICT (id, coordinator_id)
DO UPDATE SET
	id = $1,
	coordinator_id = $2,
	node = $3,
	status = $4,
	updated_at = now() at time zone 'utc'
RETURNING *;

-- name: DeleteTailnetPeer :one
DELETE
FROM tailnet_peers
WHERE id = $1 and coordinator_id = $2
RETURNING id, coordinator_id;

-- name: GetTailnetPeers :many
SELECT * FROM tailnet_peers WHERE id = $1;

-- name: UpsertTailnetTunnel :one
INSERT INTO
	tailnet_tunnels (
	coordinator_id,
	src_id,
	dst_id,
	updated_at
)
VALUES
	($1, $2, $3, now() at time zone 'utc')
ON CONFLICT (coordinator_id, src_id, dst_id)
DO UPDATE SET
	coordinator_id = $1,
	src_id = $2,
	dst_id = $3,
	updated_at = now() at time zone 'utc'
RETURNING *;

-- name: DeleteTailnetTunnel :one
DELETE
FROM tailnet_tunnels
WHERE coordinator_id = $1 and src_id = $2 and dst_id = $3
RETURNING coordinator_id, src_id, dst_id;

-- name: DeleteAllTailnetTunnels :exec
DELETE
FROM tailnet_tunnels
WHERE coordinator_id = $1 and src_id = $2;

-- name: GetTailnetTunnelPeerIDs :many
SELECT dst_id as peer_id, coordinator_id, updated_at
FROM tailnet_tunnels
WHERE tailnet_tunnels.src_id = $1
UNION
SELECT src_id as peer_id, coordinator_id, updated_at
FROM tailnet_tunnels
WHERE tailnet_tunnels.dst_id = $1;

-- name: GetTailnetTunnelPeerBindings :many
SELECT tailnet_tunnels.dst_id as peer_id, tailnet_peers.coordinator_id, tailnet_peers.updated_at, tailnet_peers.node, tailnet_peers.status
FROM tailnet_tunnels
INNER JOIN tailnet_peers ON tailnet_tunnels.dst_id = tailnet_peers.id
WHERE tailnet_tunnels.src_id = $1
UNION
SELECT tailnet_tunnels.src_id as peer_id, tailnet_peers.coordinator_id, tailnet_peers.updated_at, tailnet_peers.node, tailnet_peers.status
FROM tailnet_tunnels
INNER JOIN tailnet_peers ON tailnet_tunnels.src_id = tailnet_peers.id
WHERE tailnet_tunnels.dst_id = $1;

-- name: PublishReadyForHandshake :exec
SELECT pg_notify(
	'tailnet_ready_for_handshake',
	format('%s,%s', sqlc.arg('to')::text, sqlc.arg('from')::text)
);

-- For PG Coordinator HTMLDebug

-- name: GetAllTailnetCoordinators :many
SELECT * FROM tailnet_coordinators;

-- name: GetAllTailnetPeers :many
SELECT * FROM tailnet_peers;

-- name: GetAllTailnetTunnels :many
SELECT * FROM tailnet_tunnels;
