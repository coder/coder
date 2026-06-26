DO $$
BEGIN
    IF to_regclass('chat_providers') IS NULL THEN
        RETURN;
    END IF;

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
END $$;
