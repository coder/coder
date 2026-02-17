-- name: GetChatModelConfigByID :one
SELECT
    *
FROM
    chat_model_configs
WHERE
    id = @id::uuid;

-- name: GetChatModelConfigs :many
SELECT
    *
FROM
    chat_model_configs
ORDER BY
    provider ASC,
    model ASC;

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
    cmc.model ASC;

-- name: InsertChatModelConfig :one
INSERT INTO chat_model_configs (
    provider,
    model,
    display_name,
    enabled
) VALUES (
    @provider::text,
    @model::text,
    @display_name::text,
    @enabled::boolean
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
