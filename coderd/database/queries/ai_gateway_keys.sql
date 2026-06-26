-- name: InsertAIGatewayKey :one
INSERT INTO ai_gateway_keys (id, name, secret_prefix, hashed_secret, created_at)
VALUES ($1, @name, $2, $3, NOW())
RETURNING id, name, secret_prefix, created_at;

-- name: ListAIGatewayKeys :many
SELECT id, name, secret_prefix, created_at, last_heartbeat_at
FROM ai_gateway_keys
ORDER BY created_at ASC;

-- name: DeleteAIGatewayKey :one
DELETE FROM ai_gateway_keys WHERE id = $1
RETURNING id, name, secret_prefix, created_at, last_heartbeat_at;

-- name: GetAIGatewayKeyByHashedSecret :one
-- Authenticates a standalone AI Gateway replica by its hashed key secret,
-- returning the matched key. The lookup is an exact match on a unique index,
-- so a returned row is itself proof the secret is valid.
SELECT *
FROM ai_gateway_keys
WHERE hashed_secret = $1;

-- name: UpdateAIGatewayKeyLastHeartbeatAt :execrows
-- Records heartbeat liveness for an active Gateway DRPC session. The database sets the
-- timestamp so it stays consistent regardless of clock drift between API
-- replicas.
UPDATE ai_gateway_keys
SET last_heartbeat_at = NOW()
WHERE id = $1;
