-- name: InsertAIGatewayCoderdKey :one
INSERT INTO ai_gateway_coderd_keys (id, created_at, name, secret_prefix, hashed_secret)
VALUES ($1, $2, lower(@name), $3, $4)
RETURNING id, name, secret_prefix, created_at;

-- name: ListAIGatewayCoderdKeys :many
SELECT id, name, secret_prefix, created_at, last_used_at
FROM ai_gateway_coderd_keys
ORDER BY created_at ASC;

-- name: DeleteAIGatewayCoderdKey :exec
DELETE FROM ai_gateway_coderd_keys WHERE id = $1;
