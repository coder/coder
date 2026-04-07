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
WHERE
    cmc.enabled = TRUE
    AND cmc.deleted = FALSE
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
    options,
    provider_config_id
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
    sqlc.narg('provider_config_id')::uuid
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
    provider_config_id = sqlc.narg('provider_config_id')::uuid,
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

-- name: SoftDeleteUnboundChatModelConfigsByProvider :execrows
-- Soft-deletes model configs in the given provider family that have
-- no provider_config_id binding. Used during last-provider cleanup
-- so lingering NULL-bound rows do not become orphans.
UPDATE chat_model_configs
SET deleted = TRUE,
    deleted_at = NOW(),
    updated_at = NOW()
WHERE provider = @provider::text
  AND provider_config_id IS NULL
  AND deleted = FALSE;

-- name: SoftDeleteBoundChatModelConfigsByProviderConfigID :execrows
-- Soft-deletes model configs bound to a specific provider config.
-- Called before provider deletion so bound models are preserved
-- (soft-deleted) rather than hard-removed by a database cascade.
UPDATE chat_model_configs
SET deleted = TRUE,
    deleted_at = NOW(),
    updated_at = NOW()
WHERE provider_config_id = @provider_config_id::uuid
  AND deleted = FALSE;
