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
