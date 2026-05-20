-- name: InsertAIBridgeCoderdKey :one
INSERT INTO aibridge_coderd_keys (id, created_at, name, token_prefix, hashed_secret)
VALUES ($1, $2, lower(@name), $3, $4)
RETURNING id, name, token_prefix, created_at;

-- name: ListAIBridgeCoderdKeys :many
SELECT id, name, token_prefix, created_at, last_used_at
FROM aibridge_coderd_keys
ORDER BY created_at ASC;

-- name: DeleteAIBridgeCoderdKey :exec
DELETE FROM aibridge_coderd_keys WHERE id = $1;
