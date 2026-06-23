-- name: GetChatModelConfigByID :one
SELECT
    *
FROM
    chat_model_configs
WHERE
    id = @id::uuid
    AND deleted = FALSE;

-- name: GetDefaultChatModelConfig :one
SELECT
    *
FROM
    chat_model_configs
WHERE
    is_default = TRUE
    AND deleted = FALSE;

-- name: GetChatModelConfigs :many
SELECT
    *
FROM
    chat_model_configs
WHERE
    deleted = FALSE
ORDER BY
    provider ASC,
    model ASC,
    updated_at DESC,
    id DESC;

-- name: GetEnabledChatModelConfigs :many
SELECT
    cmc.*
FROM
    chat_model_configs cmc
JOIN
    ai_providers ap ON ap.id = cmc.ai_provider_id
WHERE
    cmc.enabled = TRUE
    AND cmc.deleted = FALSE
    AND ap.enabled = TRUE
    AND ap.deleted = FALSE
ORDER BY
    cmc.provider ASC,
    cmc.model ASC,
    cmc.updated_at DESC,
    cmc.id DESC;

-- name: GetEnabledChatModelConfigByID :one
SELECT
    cmc.*
FROM
    chat_model_configs cmc
-- Providers can be disabled independently of their model configs.
-- Check both to ensure the selected config is actually usable.
JOIN
    ai_providers ap ON ap.id = cmc.ai_provider_id
WHERE
    cmc.id = @id::uuid
    AND cmc.deleted = FALSE
    AND cmc.enabled = TRUE
    AND ap.enabled = TRUE
    AND ap.deleted = FALSE;

-- name: InsertChatModelConfig :one
INSERT INTO chat_model_configs (
    provider,
    model,
    display_name,
    created_by,
    updated_by,
    enabled,
    is_default,
    context_limit,
    compression_threshold,
    options,
    ai_provider_id
) VALUES (
    @provider::text,
    @model::text,
    @display_name::text,
    sqlc.narg('created_by')::uuid,
    sqlc.narg('updated_by')::uuid,
    @enabled::boolean,
    @is_default::boolean,
    @context_limit::bigint,
    @compression_threshold::integer,
    @options::jsonb,
    sqlc.narg('ai_provider_id')::uuid
)
RETURNING
    *;

-- name: UpdateChatModelConfig :one
UPDATE
    chat_model_configs
SET
    provider = @provider::text,
    model = @model::text,
    display_name = @display_name::text,
    updated_by = sqlc.narg('updated_by')::uuid,
    enabled = @enabled::boolean,
    is_default = @is_default::boolean,
    context_limit = @context_limit::bigint,
    compression_threshold = @compression_threshold::integer,
    options = @options::jsonb,
    ai_provider_id = sqlc.narg('ai_provider_id')::uuid,
    updated_at = NOW()
WHERE
    id = @id::uuid
    AND deleted = FALSE
RETURNING
    *;

-- name: UnsetDefaultChatModelConfigs :exec
UPDATE
    chat_model_configs
SET
    is_default = FALSE,
    updated_at = NOW()
WHERE
    is_default = TRUE
    AND deleted = FALSE;

-- name: DeleteChatModelConfigByID :exec
UPDATE
    chat_model_configs
SET
    deleted = TRUE,
    deleted_at = NOW(),
    updated_at = NOW()
WHERE
    id = @id::uuid;

-- name: DeleteChatModelConfigsByProvider :exec
UPDATE
    chat_model_configs
SET
    deleted = TRUE,
    deleted_at = NOW(),
    updated_at = NOW()
WHERE
    provider = @provider::text
    AND deleted = FALSE;

-- name: BackfillChatModelConfigProvider :execresult
-- old_provider is matched as text; new_provider is also cast to ai_provider_type
-- for the EXISTS check against ai_providers.type.
-- ai_provider_id IS NOT NULL is defensive; the check constraint already
-- enforces that non-deleted rows always have a provider ID.
UPDATE
    chat_model_configs
SET
    provider   = @new_provider::text,
    updated_at = NOW()
WHERE
    provider          = @old_provider::text
    AND deleted       = FALSE
    AND ai_provider_id IS NOT NULL
    AND EXISTS (
        SELECT 1 FROM ai_providers
        WHERE  id      = chat_model_configs.ai_provider_id
          AND  type    = @new_provider::ai_provider_type
          AND  deleted = FALSE
    );

-- name: DeleteChatModelConfigsByAIProviderID :exec
UPDATE
    chat_model_configs
SET
    deleted = TRUE,
    deleted_at = NOW(),
    updated_at = NOW()
WHERE
    ai_provider_id = @ai_provider_id::uuid
    AND deleted = FALSE;
