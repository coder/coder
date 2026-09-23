-- name: HasAIModelAccess :one
SELECT EXISTS (
    SELECT 1
    FROM organization_members om
    JOIN organizations o ON o.id = om.organization_id
    WHERE om.user_id = @user_id::uuid
      AND NOT o.deleted
      AND (
          NOT o.restrict_models_to_configured
          OR EXISTS (
              SELECT 1
              FROM chat_model_configs cmc
              JOIN ai_providers ap ON ap.id = cmc.ai_provider_id
              WHERE cmc.organization_id = o.id
                AND cmc.enabled
                AND NOT cmc.deleted
                AND cmc.model = @model::text
                AND ap.name = @provider_name::text
                AND NOT ap.deleted
          )
      )
);
