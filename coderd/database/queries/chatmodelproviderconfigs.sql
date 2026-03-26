-- name: GetModelProviderConfigs :many
-- Returns all provider config attachments for a single model, ordered by priority.
SELECT
    cmpc.*,
    cp.provider,
    cp.display_name AS provider_display_name,
    cp.enabled AS provider_enabled,
    cp.api_key AS provider_api_key,
    cp.base_url AS provider_base_url
FROM
    chat_model_provider_configs cmpc
JOIN
    chat_providers cp ON cp.id = cmpc.provider_config_id
WHERE
    cmpc.model_config_id = @model_config_id::uuid
ORDER BY
    cmpc.priority ASC;

-- name: InsertModelProviderConfig :one
INSERT INTO chat_model_provider_configs (
    model_config_id,
    provider_config_id,
    priority
) VALUES (
    @model_config_id::uuid,
    @provider_config_id::uuid,
    @priority::integer
)
RETURNING *;

-- name: DeleteModelProviderConfigsByModelID :exec
DELETE FROM chat_model_provider_configs
WHERE model_config_id = @model_config_id::uuid;

-- name: GetModelProviderConfigsByModelIDs :many
-- Batch-loads provider config attachments for multiple models.
SELECT
    cmpc.*,
    cp.provider,
    cp.display_name AS provider_display_name,
    cp.enabled AS provider_enabled,
    cp.api_key AS provider_api_key,
    cp.base_url AS provider_base_url
FROM
    chat_model_provider_configs cmpc
JOIN
    chat_providers cp ON cp.id = cmpc.provider_config_id
WHERE
    cmpc.model_config_id = ANY(@model_config_ids::uuid[])
ORDER BY
    cmpc.model_config_id,
    cmpc.priority ASC;
