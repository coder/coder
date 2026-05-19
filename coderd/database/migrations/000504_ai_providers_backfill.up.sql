-- Override any pre-existing live AI providers whose names collide with the
-- backfill below. No other process should write to ai_providers before this
-- migration, so any conflicting live row is treated as stale and soft-deleted
-- to free the name for the chat_providers row inserted below, which becomes
-- authoritative.
UPDATE ai_providers
SET deleted = TRUE,
    enabled = FALSE,
    updated_at = NOW()
WHERE deleted = FALSE
    AND name IN (
        SELECT 'agents-' || cp.provider
        FROM chat_providers cp
    );

INSERT INTO ai_providers (
    id,
    type,
    name,
    display_name,
    enabled,
    base_url,
    created_at,
    updated_at
)
SELECT
    cp.id,
    cp.provider::ai_provider_type,
    'agents-' || cp.provider,
    NULLIF(cp.display_name, ''),
    cp.enabled,
    cp.base_url,
    cp.created_at,
    cp.updated_at
FROM chat_providers cp;

INSERT INTO ai_provider_keys (
    id,
    provider_id,
    api_key,
    api_key_key_id,
    created_at,
    updated_at
)
SELECT
    gen_random_uuid(),
    cp.id,
    cp.api_key,
    cp.api_key_key_id,
    cp.created_at,
    cp.updated_at
FROM chat_providers cp
WHERE cp.api_key != '';

CREATE OR REPLACE FUNCTION sync_chat_provider_to_ai_provider() RETURNS trigger
    LANGUAGE plpgsql
AS $$
DECLARE
    provider_key_id uuid;
BEGIN
    IF (TG_OP = 'DELETE') THEN
        UPDATE ai_providers
        SET
            enabled = FALSE,
            deleted = TRUE,
            updated_at = NOW()
        WHERE id = OLD.id;

        DELETE FROM ai_provider_keys
        WHERE provider_id = OLD.id;

        RETURN OLD;
    END IF;

    INSERT INTO ai_providers (
        id,
        type,
        name,
        display_name,
        enabled,
        base_url,
        created_at,
        updated_at,
        deleted
    ) VALUES (
        NEW.id,
        NEW.provider::ai_provider_type,
        'agents-' || NEW.provider,
        NULLIF(NEW.display_name, ''),
        NEW.enabled,
        NEW.base_url,
        NEW.created_at,
        NEW.updated_at,
        FALSE
    )
    ON CONFLICT (id) DO UPDATE
    SET
        type = EXCLUDED.type,
        name = EXCLUDED.name,
        display_name = EXCLUDED.display_name,
        enabled = EXCLUDED.enabled,
        base_url = EXCLUDED.base_url,
        updated_at = EXCLUDED.updated_at,
        deleted = FALSE;

    SELECT apk.id INTO provider_key_id
    FROM ai_provider_keys apk
    WHERE apk.provider_id = NEW.id
    ORDER BY apk.created_at ASC, apk.id ASC
    LIMIT 1;

    IF (NEW.api_key = '') THEN
        IF provider_key_id IS NOT NULL THEN
            DELETE FROM ai_provider_keys
            WHERE id = provider_key_id;
        END IF;
        RETURN NEW;
    END IF;

    IF provider_key_id IS NULL THEN
        INSERT INTO ai_provider_keys (
            id,
            provider_id,
            api_key,
            api_key_key_id,
            created_at,
            updated_at
        ) VALUES (
            gen_random_uuid(),
            NEW.id,
            NEW.api_key,
            NEW.api_key_key_id,
            NEW.created_at,
            NEW.updated_at
        );
    ELSE
        UPDATE ai_provider_keys
        SET
            api_key = NEW.api_key,
            api_key_key_id = NEW.api_key_key_id,
            updated_at = NEW.updated_at
        WHERE id = provider_key_id;
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER sync_chat_provider_to_ai_provider
    AFTER INSERT OR UPDATE OR DELETE ON chat_providers
    FOR EACH ROW EXECUTE FUNCTION sync_chat_provider_to_ai_provider();

INSERT INTO user_ai_provider_keys (
    id,
    user_id,
    ai_provider_id,
    api_key,
    api_key_key_id,
    created_at,
    updated_at
)
SELECT
    ucpk.id,
    ucpk.user_id,
    ucpk.chat_provider_id,
    ucpk.api_key,
    ucpk.api_key_key_id,
    ucpk.created_at,
    ucpk.updated_at
FROM user_chat_provider_keys ucpk;

CREATE OR REPLACE FUNCTION sync_user_chat_provider_key_to_ai_provider_key() RETURNS trigger
    LANGUAGE plpgsql
AS $$
BEGIN
    IF (TG_OP = 'DELETE') THEN
        DELETE FROM user_ai_provider_keys
        WHERE user_id = OLD.user_id
            AND ai_provider_id = OLD.chat_provider_id;
        RETURN OLD;
    END IF;

    INSERT INTO user_ai_provider_keys (
        id,
        user_id,
        ai_provider_id,
        api_key,
        api_key_key_id,
        created_at,
        updated_at
    ) VALUES (
        NEW.id,
        NEW.user_id,
        NEW.chat_provider_id,
        NEW.api_key,
        NEW.api_key_key_id,
        NEW.created_at,
        NEW.updated_at
    )
    ON CONFLICT (user_id, ai_provider_id) DO UPDATE
    SET
        api_key = EXCLUDED.api_key,
        api_key_key_id = EXCLUDED.api_key_key_id,
        updated_at = EXCLUDED.updated_at;

    RETURN NEW;
END;
$$;

CREATE TRIGGER sync_user_chat_provider_key_to_ai_provider_key
    AFTER INSERT OR UPDATE OR DELETE ON user_chat_provider_keys
    FOR EACH ROW EXECUTE FUNCTION sync_user_chat_provider_key_to_ai_provider_key();

UPDATE chat_model_configs cmc
SET ai_provider_id = cp.id
FROM chat_providers cp
WHERE cmc.provider = cp.provider
    AND cmc.ai_provider_id IS NULL;
