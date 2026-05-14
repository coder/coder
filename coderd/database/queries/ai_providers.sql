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

-- name: GetAIProviders :many
-- Returns AI provider rows. Soft-deleted and/or disabled rows are
-- excluded by default; callers pass include_deleted=true to also see
-- soft-deleted rows (the env seeder uses this to distinguish "never
-- existed" from "operator soft-deleted; do not re-create from env")
-- and include_disabled=true to also see rows the operator has marked
-- disabled.
SELECT
    *
FROM
    ai_providers
WHERE
    (@include_deleted::boolean OR NOT deleted)
    AND (@include_disabled::boolean OR enabled)
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
    sqlc.narg('display_name')::text,
    @enabled::boolean,
    @base_url::text,
    sqlc.narg('settings')::text,
    sqlc.narg('settings_key_id')::text
)
RETURNING
    *;

-- name: UpdateAIProvider :one
UPDATE
    ai_providers
SET
    display_name = sqlc.narg('display_name')::text,
    enabled = @enabled::boolean,
    base_url = @base_url::text,
    settings = sqlc.narg('settings')::text,
    settings_key_id = sqlc.narg('settings_key_id')::text,
    updated_at = NOW()
WHERE
    id = @id::uuid AND deleted = FALSE
RETURNING
    *;

-- name: DeleteAIProviderByID :exec
UPDATE
    ai_providers
SET
    deleted = TRUE,
    enabled = FALSE,
    updated_at = NOW()
WHERE
    id = @id::uuid AND deleted = FALSE;

-- name: UpdateAIProviderSettings :one
-- Updates only the settings columns (settings, settings_key_id) and
-- the updated_at timestamp on a row, regardless of its deleted flag.
-- Used by the dbcrypt key rotation utility to re-encrypt or decrypt
-- rows in place.
UPDATE
    ai_providers
SET
    settings = sqlc.narg('settings')::text,
    settings_key_id = sqlc.narg('settings_key_id')::text,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;
