-- name: GetAIProviderKeyByID :one
SELECT
    *
FROM
    ai_provider_keys
WHERE
    id = @id::uuid;

-- name: GetAIProviderKeysByProviderID :many
-- Returns all keys for a provider, ordered by created_at ASC so the
-- oldest key is returned first. AI Bridge currently uses the oldest
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

-- name: GetAIProviderKeyPresence :many
-- Returns the provider IDs that have at least one provider-scoped key.
SELECT DISTINCT
    provider_id
FROM
    ai_provider_keys
WHERE
    provider_id = ANY(@provider_ids::uuid[])
ORDER BY
    provider_id ASC;

-- name: GetAIProviderKeysByProviderIDs :many
-- Returns all keys for the requested providers, ordered by provider then created_at ASC
-- so callers can select the oldest non-empty key per provider without issuing N queries.
SELECT
    *
FROM
    ai_provider_keys
WHERE
    provider_id = ANY(@provider_ids::uuid[])
ORDER BY
    provider_id ASC,
    created_at ASC,
    id ASC;

-- name: GetAIProviderKeys :many
-- Returns AI provider key rows. By default, only rows whose parent
-- provider is live (deleted = FALSE) are returned, so the API list
-- handler can fetch every visible provider's keys in a single query.
-- The dbcrypt key rotation utility passes include_deleted=TRUE to
-- re-encrypt rows that belong to soft-deleted providers as well.
SELECT
    ai_provider_keys.*
FROM
    ai_provider_keys
    JOIN ai_providers ON ai_providers.id = ai_provider_keys.provider_id
WHERE
    @include_deleted::boolean OR NOT ai_providers.deleted
ORDER BY
    ai_provider_keys.provider_id ASC,
    ai_provider_keys.created_at ASC,
    ai_provider_keys.id ASC;

-- name: InsertAIProviderKey :one
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
    @created_at::timestamptz,
    @updated_at::timestamptz
)
RETURNING
    *;

-- name: DeleteAIProviderKey :exec
DELETE FROM
    ai_provider_keys
WHERE
    id = @id::uuid;

-- name: UpdateEncryptedAIProviderKey :one
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
