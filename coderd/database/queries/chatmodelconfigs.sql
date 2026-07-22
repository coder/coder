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
    cmc.*
FROM
    chat_model_configs cmc
LEFT JOIN
    ai_providers ap ON ap.id = cmc.ai_provider_id
WHERE
    cmc.deleted = FALSE
ORDER BY
    ap.type::text ASC,
    cmc.model ASC,
    cmc.updated_at DESC,
    cmc.id DESC;

-- name: GetEnabledChatModelConfigs :many
SELECT
    sqlc.embed(cmc),
    ap.type::text AS provider
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
    ap.type::text ASC,
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
