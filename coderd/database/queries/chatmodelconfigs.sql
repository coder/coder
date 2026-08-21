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
    AND deleted = FALSE
    AND organization_id = @organization_id::uuid;

-- name: GetChatModelConfigs :many
SELECT
    cmc.*
FROM
    chat_model_configs cmc
LEFT JOIN
    ai_providers ap ON ap.id = cmc.ai_provider_id
WHERE
    cmc.deleted = FALSE
    -- Authorize Filter clause will be injected below in GetAuthorizedChatModelConfigs
    -- @authorize_filter
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

-- name: GetEnabledChatModelConfigsByOrganization :many
SELECT
    sqlc.embed(cmc),
    ap.type::text AS provider
FROM
    chat_model_configs cmc
JOIN
    ai_providers ap ON ap.id = cmc.ai_provider_id
WHERE
    cmc.organization_id = @organization_id::uuid
    AND cmc.enabled = TRUE
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
    ai_provider_id,
    organization_id,
    group_acl,
    user_acl
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
    sqlc.narg('ai_provider_id')::uuid,
    @organization_id::uuid,
    @group_acl,
    @user_acl
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

-- name: UpdateChatModelConfigACLByID :one
UPDATE
    chat_model_configs
SET
    group_acl = @group_acl,
    user_acl = @user_acl,
    updated_by = sqlc.narg('updated_by')::uuid,
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
    AND deleted = FALSE
    AND organization_id = @organization_id::uuid;

-- name: DeleteChatModelConfigByID :one
UPDATE
    chat_model_configs
SET
    deleted = TRUE,
    deleted_at = NOW(),
    updated_at = NOW()
WHERE
    id = @id::uuid
    AND deleted = FALSE
RETURNING id;
