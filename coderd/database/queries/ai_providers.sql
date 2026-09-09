-- name: GetAIProviderByID :one
SELECT
    *
FROM
    ai_providers
WHERE
    id = @id::uuid AND deleted = FALSE;

-- name: GetAIProviderByIDForReferenceLock :one
SELECT
    *
FROM
    ai_providers
WHERE
    id = @id::uuid AND deleted = FALSE
-- Lock the provider row until the model-config write completes. The
-- transaction alone does not stop a concurrent soft-delete or disable
-- between validation and writing the model config reference.
FOR SHARE;

-- name: GetAIProviderByName :one
SELECT
    *
FROM
    ai_providers
WHERE
    name = @name::text AND deleted = FALSE;

-- name: GetAIProviders :many
-- Returns AI provider rows. Soft-deleted and disabled rows are excluded
-- unless include_deleted or include_disabled is set.
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
    icon,
    enabled,
    base_url,
    settings,
    settings_key_id
) VALUES (
    @id::uuid,
    @type::ai_provider_type,
    @name::text,
    sqlc.narg('display_name')::text,
    @icon::text,
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
    type = @type::ai_provider_type,
    display_name = sqlc.narg('display_name')::text,
    icon = @icon::text,
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

-- name: UpdateEncryptedAIProviderSettings :one
-- Updates only the encrypted columns (settings, settings_key_id) and
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

-- name: UpsertAIProviderBedrockResolvedModels :exec
-- Records the models an application inference profile ARN resolves to. The
-- provider write path resolves the ARN through the Bedrock control plane and
-- stores the answer here, so the gateway never has to.
INSERT INTO
    ai_provider_bedrock_resolved_models (ai_provider_id, resolved_model, resolved_small_fast_model)
VALUES
    (@ai_provider_id::uuid, @resolved_model::text, @resolved_small_fast_model::text)
ON CONFLICT (ai_provider_id) DO UPDATE SET
    resolved_model = @resolved_model::text,
    resolved_small_fast_model = @resolved_small_fast_model::text;

-- name: DeleteAIProviderBedrockResolvedModels :exec
DELETE FROM
    ai_provider_bedrock_resolved_models
WHERE
    ai_provider_id = @ai_provider_id::uuid;

-- name: GetAIProviderBedrockResolvedModelsByProviderIDs :many
SELECT
    *
FROM
    ai_provider_bedrock_resolved_models
WHERE
    ai_provider_id = ANY(@ai_provider_ids::uuid[]);
