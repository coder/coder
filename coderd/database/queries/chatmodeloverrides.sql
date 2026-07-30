-- Chat model override storage (CODAGT-74, unit U4). Presence means pinned;
-- absence means unset. The composite FK to
-- chat_model_configs(organization_id, id) binds every pin to a config in
-- the same organization, so no query here needs a cross-organization guard.

-- GetChatOrganizationModelOverrides returns every pinned admin override for
-- one organization, one row per context.
-- name: GetChatOrganizationModelOverrides :many
SELECT id, organization_id, context, model_config_id, reasoning_effort
FROM chat_organization_model_overrides
WHERE organization_id = @organization_id
ORDER BY context;

-- GetChatOrganizationModelOverride returns one pinned admin override, or
-- sql.ErrNoRows when the (organization, context) pair is unset.
-- name: GetChatOrganizationModelOverride :one
SELECT id, organization_id, context, model_config_id, reasoning_effort
FROM chat_organization_model_overrides
WHERE organization_id = @organization_id
  AND context = @context;

-- UpsertChatOrganizationModelOverride pins a model config for one
-- (organization, context). Only called after validation, so model_config_id
-- is always non-NULL here; clearing an override is DeleteChatOrganizationModelOverride.
-- name: UpsertChatOrganizationModelOverride :exec
INSERT INTO chat_organization_model_overrides
    (organization_id, context, model_config_id, reasoning_effort)
VALUES (@organization_id, @context, @model_config_id, @reasoning_effort)
ON CONFLICT ON CONSTRAINT chat_organization_model_overrides_organization_id_context_key
DO UPDATE SET
    model_config_id = EXCLUDED.model_config_id,
    reasoning_effort = EXCLUDED.reasoning_effort;

-- DeleteChatOrganizationModelOverride clears one (organization, context)
-- pin. Clearing an unset context deletes zero rows, which is not an error.
-- name: DeleteChatOrganizationModelOverride :exec
DELETE FROM chat_organization_model_overrides
WHERE organization_id = @organization_id
  AND context = @context;

-- GetChatUserModelOverrides returns one user's personal override rows for
-- one organization, including explicit sentinel-mode rows (chat_default,
-- deployment_default), which are real rows with NULL model_config_id.
-- name: GetChatUserModelOverrides :many
SELECT id, user_id, organization_id, context, mode, model_config_id, reasoning_effort
FROM chat_user_model_overrides
WHERE user_id = @user_id
  AND organization_id = @organization_id
ORDER BY context;

-- GetChatUserModelOverride returns one user's personal override for one
-- (organization, context), or sql.ErrNoRows when unset.
-- name: GetChatUserModelOverride :one
SELECT id, user_id, organization_id, context, mode, model_config_id, reasoning_effort
FROM chat_user_model_overrides
WHERE user_id = @user_id
  AND organization_id = @organization_id
  AND context = @context;

-- UpsertChatUserModelOverride records one user's personal override. Sentinel
-- modes write NULL model_config_id/reasoning_effort (an explicit user
-- choice, distinct from absence); mode 'model' writes the pin.
-- name: UpsertChatUserModelOverride :exec
INSERT INTO chat_user_model_overrides
    (user_id, organization_id, context, mode, model_config_id, reasoning_effort)
VALUES (@user_id, @organization_id, @context, @mode, @model_config_id, @reasoning_effort)
ON CONFLICT ON CONSTRAINT chat_user_model_overrides_user_organization_context_key
DO UPDATE SET
    mode = EXCLUDED.mode,
    model_config_id = EXCLUDED.model_config_id,
    reasoning_effort = EXCLUDED.reasoning_effort;
