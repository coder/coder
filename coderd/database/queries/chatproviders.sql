-- name: GetChatProviderByID :one
SELECT
    *
FROM
    chat_providers
WHERE
    id = @id::uuid;

-- name: GetEnabledChatProviderByProvider :one
-- Returns the oldest enabled provider config for a given provider family.
-- Multiple enabled configs may exist per family; this returns the
-- first-created one.
SELECT
    *
FROM
    chat_providers
WHERE
    provider = @provider::text
    AND enabled = TRUE
ORDER BY
    created_at ASC,
    id ASC
LIMIT 1;

-- name: GetChatProviders :many
SELECT
    *
FROM
    chat_providers
ORDER BY
    provider ASC,
    created_at ASC,
    id ASC;

-- name: GetEnabledChatProviders :many
SELECT
    *
FROM
    chat_providers
WHERE
    enabled = TRUE
ORDER BY
    provider ASC,
    created_at ASC,
    id ASC;

-- name: InsertChatProvider :one
INSERT INTO chat_providers (
    provider,
    display_name,
    api_key,
    base_url,
    api_key_key_id,
    created_by,
    enabled,
    central_api_key_enabled,
    allow_user_api_key,
    allow_central_api_key_fallback
) VALUES (
    @provider::text,
    @display_name::text,
    @api_key::text,
    @base_url::text,
    sqlc.narg('api_key_key_id')::text,
    sqlc.narg('created_by')::uuid,
    @enabled::boolean,
    @central_api_key_enabled::boolean,
    @allow_user_api_key::boolean,
    @allow_central_api_key_fallback::boolean
)
RETURNING
    *;

-- name: UpdateChatProvider :one
UPDATE
    chat_providers
SET
    display_name = @display_name::text,
    api_key = @api_key::text,
    base_url = @base_url::text,
    api_key_key_id = sqlc.narg('api_key_key_id')::text,
    enabled = @enabled::boolean,
    central_api_key_enabled = @central_api_key_enabled::boolean,
    allow_user_api_key = @allow_user_api_key::boolean,
    allow_central_api_key_fallback = @allow_central_api_key_fallback::boolean,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: DeleteChatProviderByID :exec
DELETE FROM
    chat_providers
WHERE
    id = @id::uuid;

-- name: CountChatProvidersByProviderExcludingID :one
-- Counts remaining provider configs in the same family, excluding
-- the target row. Used by deleteChatProvider to detect last-provider
-- deletes.
SELECT
    COUNT(*)::int
FROM
    chat_providers
WHERE
    provider = @provider::text
    AND id != @id::uuid;
