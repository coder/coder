-- name: GetChatProviderByID :one
SELECT
    *
FROM
    chat_providers
WHERE
    id = @id::uuid;

-- name: GetChatProviderByProvider :one
SELECT
    *
FROM
    chat_providers
WHERE
    provider = @provider::text;

-- name: GetChatProviders :many
SELECT
    *
FROM
    chat_providers
ORDER BY
    provider ASC;

-- name: GetEnabledChatProviders :many
SELECT
    *
FROM
    chat_providers
WHERE
    enabled = TRUE
ORDER BY
    provider ASC;

-- name: InsertChatProvider :one
INSERT INTO chat_providers (
    provider,
    display_name,
    api_key,
    base_url,
    api_key_key_id,
    created_by,
    enabled
) VALUES (
    @provider::text,
    @display_name::text,
    @api_key::text,
    @base_url::text,
    sqlc.narg('api_key_key_id')::text,
    sqlc.narg('created_by')::uuid,
    @enabled::boolean
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
