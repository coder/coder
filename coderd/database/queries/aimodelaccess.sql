-- name: GetAIModelAccessConfigs :many
SELECT
    cmc.id,
    cmc.organization_id
FROM organization_members om
JOIN organizations o ON o.id = om.organization_id
JOIN chat_model_configs cmc ON cmc.organization_id = o.id
JOIN ai_providers ap ON ap.id = cmc.ai_provider_id
WHERE om.user_id = @user_id::uuid
  AND NOT o.deleted
  AND cmc.enabled
  AND NOT cmc.deleted
  AND NOT ap.deleted
  AND ap.name = @provider_name::text
  AND cmc.model = @model::text;
