-- name: InsertAIGatewayCoderdKey :one
INSERT INTO ai_gateway_coderd_keys (id, name, secret_prefix, hashed_secret, created_at)
VALUES ($1, @name, $2, $3, NOW())
RETURNING id, name, secret_prefix, created_at;

-- name: ListAIGatewayCoderdKeys :many
SELECT id, name, secret_prefix, created_at, last_used_at
FROM ai_gateway_coderd_keys
ORDER BY created_at ASC;

-- name: DeleteAIGatewayCoderdKey :one
DELETE FROM ai_gateway_coderd_keys WHERE id = $1 RETURNING id;
