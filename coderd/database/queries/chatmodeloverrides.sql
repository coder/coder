-- name: GetChatOrganizationModelOverrides :many
SELECT id, organization_id, context, model_config_id, reasoning_effort
FROM chat_organization_model_overrides
WHERE organization_id = @organization_id
ORDER BY context;

-- name: GetChatOrganizationModelOverride :one
SELECT id, organization_id, context, model_config_id, reasoning_effort
FROM chat_organization_model_overrides
WHERE organization_id = @organization_id
  AND context = @context;

-- name: GetChatOrganizationModelOverridesByContext :many
-- Returns every non-deleted organization's override for one context together
-- with the resolved model and provider, for bulk consumers such as telemetry.
-- model_available mirrors GetEnabledChatModelConfigByID: it is false when the
-- referenced config or its provider is disabled or deleted.
SELECT
    o.organization_id,
    (cmc.id IS NOT NULL AND ap.id IS NOT NULL)::boolean AS model_available,
    COALESCE(cmc.model, '')::text AS model,
    COALESCE(ap.type::text, '')::text AS provider_type
FROM chat_organization_model_overrides o
JOIN organizations org ON org.id = o.organization_id AND NOT org.deleted
LEFT JOIN chat_model_configs cmc
    ON cmc.id = o.model_config_id
    AND cmc.deleted = FALSE
    AND cmc.enabled = TRUE
LEFT JOIN ai_providers ap
    ON ap.id = cmc.ai_provider_id
    AND ap.enabled = TRUE
    AND ap.deleted = FALSE
WHERE o.context = @context
ORDER BY o.organization_id;

-- name: UpsertChatOrganizationModelOverride :exec
INSERT INTO chat_organization_model_overrides
    (organization_id, context, model_config_id, reasoning_effort)
VALUES (@organization_id, @context, @model_config_id, @reasoning_effort)
ON CONFLICT ON CONSTRAINT chat_organization_model_overrides_organization_id_context_key
DO UPDATE SET
    model_config_id = EXCLUDED.model_config_id,
    reasoning_effort = EXCLUDED.reasoning_effort;

-- name: DeleteChatOrganizationModelOverride :exec
DELETE FROM chat_organization_model_overrides
WHERE organization_id = @organization_id
  AND context = @context;

-- name: GetChatUserModelOverrides :many
SELECT id, user_id, organization_id, context, mode, model_config_id, reasoning_effort
FROM chat_user_model_overrides
WHERE user_id = @user_id
  AND organization_id = @organization_id
ORDER BY context;

-- name: GetChatUserModelOverride :one
SELECT id, user_id, organization_id, context, mode, model_config_id, reasoning_effort
FROM chat_user_model_overrides
WHERE user_id = @user_id
  AND organization_id = @organization_id
  AND context = @context;

-- name: UpsertChatUserModelOverride :exec
INSERT INTO chat_user_model_overrides
    (user_id, organization_id, context, mode, model_config_id, reasoning_effort)
VALUES (@user_id, @organization_id, @context, @mode, @model_config_id, @reasoning_effort)
ON CONFLICT ON CONSTRAINT chat_user_model_overrides_user_organization_context_key
DO UPDATE SET
    mode = EXCLUDED.mode,
    model_config_id = EXCLUDED.model_config_id,
    reasoning_effort = EXCLUDED.reasoning_effort;
