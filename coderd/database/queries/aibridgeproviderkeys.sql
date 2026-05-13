-- name: GetAIProviderKeyByID :one
SELECT
    *
FROM
    ai_provider_keys
WHERE
    id = @id::uuid;

-- name: GetAIProviderKeysByProviderID :many
-- Returns all keys for a provider, ordered by created_at ASC so the
-- oldest key is returned first. AI Bridge currently uses the first
-- key per provider; multiple keys are stored to support future
-- failover and rotation flows.
SELECT
    *
FROM
    ai_provider_keys
WHERE
    provider_id = @provider_id::uuid
ORDER BY
    created_at ASC,
    id ASC;

-- name: InsertAIProviderKey :one
-- Optional created_at allows callers (e.g. env-driven seeding) to
-- preserve insertion order across multiple rows in the same
-- transaction, where the default NOW() would otherwise tie. When
-- NULL the column falls back to its DEFAULT (NOW()).
INSERT INTO ai_provider_keys (
    id,
    provider_id,
    api_key,
    api_key_key_id,
    created_at,
    updated_at
) VALUES (
    @id::uuid,
    @provider_id::uuid,
    @api_key::text,
    sqlc.narg('api_key_key_id')::text,
    COALESCE(sqlc.narg('created_at')::timestamptz, NOW()),
    COALESCE(sqlc.narg('created_at')::timestamptz, NOW())
)
RETURNING
    *;

-- name: DeleteAIProviderKey :one
DELETE FROM
    ai_provider_keys
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: GetAIProviderKeysForRotation :many
-- Returns every AI provider key row, even those whose provider has
-- been soft-deleted, so the dbcrypt key rotation utility can
-- re-encrypt their api_key and clear references to retired keys.
SELECT
    *
FROM
    ai_provider_keys
ORDER BY
    provider_id ASC,
    created_at ASC,
    id ASC;

-- name: UpdateAIProviderKeyEncryptedColumns :one
-- Updates only the encrypted columns (api_key, api_key_key_id) and
-- the updated_at timestamp on a row. Used by the dbcrypt key
-- rotation utility to re-encrypt or decrypt rows in place.
UPDATE
    ai_provider_keys
SET
    api_key = @api_key::text,
    api_key_key_id = sqlc.narg('api_key_key_id')::text,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;
