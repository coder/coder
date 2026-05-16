DROP TRIGGER IF EXISTS sync_chat_provider_to_ai_provider ON chat_providers;
DROP FUNCTION IF EXISTS sync_chat_provider_to_ai_provider();

DROP TRIGGER IF EXISTS sync_user_chat_provider_key_to_ai_provider_key ON user_chat_provider_keys;
DROP FUNCTION IF EXISTS sync_user_chat_provider_key_to_ai_provider_key();

WITH migrated_provider_ids AS (
    SELECT id
    FROM chat_providers
    UNION
    SELECT id
    FROM ai_providers
    WHERE name LIKE 'agents-%'
        AND deleted = TRUE
)
UPDATE chat_model_configs
SET ai_provider_id = NULL
WHERE ai_provider_id IN (SELECT id FROM migrated_provider_ids);

WITH migrated_provider_ids AS (
    SELECT id
    FROM chat_providers
    UNION
    SELECT id
    FROM ai_providers
    WHERE name LIKE 'agents-%'
        AND deleted = TRUE
)
DELETE FROM user_ai_provider_keys
WHERE ai_provider_id IN (SELECT id FROM migrated_provider_ids);

WITH migrated_provider_ids AS (
    SELECT id
    FROM chat_providers
    UNION
    SELECT id
    FROM ai_providers
    WHERE name LIKE 'agents-%'
        AND deleted = TRUE
)
DELETE FROM ai_provider_keys
WHERE provider_id IN (SELECT id FROM migrated_provider_ids);

WITH migrated_provider_ids AS (
    SELECT id
    FROM chat_providers
    UNION
    SELECT id
    FROM ai_providers
    WHERE name LIKE 'agents-%'
        AND deleted = TRUE
)
DELETE FROM ai_providers
WHERE id IN (SELECT id FROM migrated_provider_ids);
