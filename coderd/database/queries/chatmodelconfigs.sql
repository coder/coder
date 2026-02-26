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

-- name: GetChatModelConfigByProviderAndModel :one
SELECT
    *
FROM
    chat_model_configs
WHERE
    provider = @provider::text
    AND model = @model::text
    AND deleted = FALSE
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
    chat_providers cp ON cp.provider = cmc.provider
WHERE
    cmc.enabled = TRUE
    AND cmc.deleted = FALSE
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
    created_by,
    updated_by,
    enabled,
    is_default,
    context_limit,
    compression_threshold,
    options
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
    @options::jsonb
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
