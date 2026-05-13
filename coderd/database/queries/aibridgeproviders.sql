-- name: GetAIProviderByID :one
SELECT
    *
FROM
    ai_providers
WHERE
    id = @id::uuid AND deleted = FALSE;

-- name: GetAIProviderByName :one
SELECT
    *
FROM
    ai_providers
WHERE
    name = @name::text AND deleted = FALSE;

-- name: GetAIProviderByNameIncludeDeleted :one
SELECT
    *
FROM
    ai_providers
WHERE
    name = @name::text;

-- name: GetAIProviders :many
SELECT
    *
FROM
    ai_providers
WHERE
    deleted = FALSE
ORDER BY
    name ASC;

-- name: GetEnabledAIProviders :many
SELECT
    *
FROM
    ai_providers
WHERE
    enabled = TRUE AND deleted = FALSE
ORDER BY
    name ASC;

-- name: InsertAIProvider :one
INSERT INTO ai_providers (
    id,
    type,
    name,
    display_name,
    enabled,
    base_url,
    settings,
    settings_key_id
) VALUES (
    @id::uuid,
    @type::ai_provider_type,
    @name::text,
    @display_name::text,
    @enabled::boolean,
    @base_url::text,
    @settings::text,
    sqlc.narg('settings_key_id')::text
)
RETURNING
    *;

-- name: UpdateAIProvider :one
UPDATE
    ai_providers
SET
    display_name = @display_name::text,
    enabled = @enabled::boolean,
    base_url = @base_url::text,
    settings = @settings::text,
    settings_key_id = sqlc.narg('settings_key_id')::text,
    updated_at = NOW()
WHERE
    id = @id::uuid AND deleted = FALSE
RETURNING
    *;

-- name: SoftDeleteAIProviderByID :one
UPDATE
    ai_providers
SET
    deleted = TRUE,
    enabled = FALSE,
    updated_at = NOW()
WHERE
    id = @id::uuid AND deleted = FALSE
RETURNING
    *;

-- name: GetAIProvidersForRotation :many
-- Returns every AI provider row, including soft-deleted ones, so the
-- dbcrypt key rotation utility can re-encrypt their settings and
-- clear references to retired keys.
SELECT
    *
FROM
    ai_providers
ORDER BY
    name ASC;

-- name: UpdateAIProviderEncryptedColumns :one
-- Updates only the encrypted columns (settings, settings_key_id) and
-- the updated_at timestamp on a row, regardless of its deleted flag.
-- Used by the dbcrypt key rotation utility to re-encrypt or decrypt
-- rows in place.
UPDATE
    ai_providers
SET
    settings = @settings::text,
    settings_key_id = sqlc.narg('settings_key_id')::text,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;
