-- name: GetChatModelConfigByID :one
SELECT
    *
FROM
    chat_model_configs
WHERE
    id = @id::uuid;

-- name: GetChatModelConfigByProviderAndModel :one
SELECT
    *
FROM
    chat_model_configs
WHERE
    provider = @provider::text
    AND model = @model::text
ORDER BY
    updated_at DESC,
    created_at DESC,
    id DESC
LIMIT 1;

-- name: GetChatModelConfigs :many
SELECT
    *
FROM
    chat_model_configs
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
    chat_providers cp ON cp.provider = cmc.provider
WHERE
    cmc.enabled = TRUE
    AND cp.enabled = TRUE
ORDER BY
    cmc.provider ASC,
    cmc.model ASC,
    cmc.updated_at DESC,
    cmc.id DESC;

-- name: InsertChatModelConfig :one
INSERT INTO chat_model_configs (
    provider,
    model,
    display_name,
    enabled,
    context_limit,
    compression_threshold,
    model_config
) VALUES (
    @provider::text,
    @model::text,
    @display_name::text,
    @enabled::boolean,
    @context_limit::bigint,
    @compression_threshold::integer,
    @model_config::jsonb
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
    enabled = @enabled::boolean,
    context_limit = @context_limit::bigint,
    compression_threshold = @compression_threshold::integer,
    model_config = @model_config::jsonb,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: DeleteChatModelConfigByID :exec
DELETE FROM
    chat_model_configs
WHERE
    id = @id::uuid;
