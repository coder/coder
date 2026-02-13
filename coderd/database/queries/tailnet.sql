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

-- name: UpdateTailnetPeerStatusByCoordinator :exec
UPDATE
	tailnet_peers
SET
	status = $2
WHERE
	coordinator_id = $1;

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
SELECT id AS peer_id, coordinator_id, updated_at, node, status
FROM tailnet_peers
WHERE id IN (
  SELECT dst_id as peer_id
  FROM tailnet_tunnels
  WHERE tailnet_tunnels.src_id = $1
  UNION
  SELECT src_id as peer_id
  FROM tailnet_tunnels
  WHERE tailnet_tunnels.dst_id = $1
);

-- For PG Coordinator HTMLDebug

-- name: GetAllTailnetCoordinators :many
SELECT * FROM tailnet_coordinators;

-- name: GetAllTailnetPeers :many
SELECT * FROM tailnet_peers;

-- name: GetAllTailnetTunnels :many
SELECT * FROM tailnet_tunnels;
