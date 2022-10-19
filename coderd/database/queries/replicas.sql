-- name: GetReplicasUpdatedAfter :many
SELECT * FROM replicas WHERE updated_at > $1 AND stopped_at IS NULL;

-- name: InsertReplica :one
INSERT INTO replicas (
    id,
    created_at,
    started_at,
    updated_at,
    hostname,
    region_id,
    relay_address,
    version,
    database_latency
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: UpdateReplica :one
UPDATE replicas SET
    updated_at = $2,
    started_at = $3,
    stopped_at = $4,
    relay_address = $5,
    region_id = $6,
    hostname = $7,
    version = $8,
    error = $9,
    database_latency = $10
WHERE id = $1 RETURNING *;

-- name: DeleteReplicasUpdatedBefore :exec
DELETE FROM replicas WHERE updated_at < $1;
