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
-- Returns AI provider rows, optionally filtered by enabled and/or
-- deleted flags. Pass NULL for either flag to skip that filter; the
-- dbcrypt key rotation utility relies on this to iterate every row,
-- including soft-deleted ones.
SELECT
    *
FROM
    ai_providers
WHERE
    CASE
        WHEN sqlc.narg('enabled')::boolean IS NULL THEN TRUE
        ELSE enabled = sqlc.narg('enabled')::boolean
    END
    AND CASE
        WHEN sqlc.narg('deleted')::boolean IS NULL THEN TRUE
        ELSE deleted = sqlc.narg('deleted')::boolean
    END
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
    sqlc.narg('settings')::text,
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
    settings = sqlc.narg('settings')::text,
    settings_key_id = sqlc.narg('settings_key_id')::text,
    updated_at = NOW()
WHERE
    id = @id::uuid AND deleted = FALSE
RETURNING
    *;

-- name: DeleteAIProviderByID :one
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
