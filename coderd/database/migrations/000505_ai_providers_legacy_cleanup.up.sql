DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM chat_providers cp
        JOIN ai_providers ap ON ap.name = 'agents-' || cp.provider
        WHERE ap.deleted = FALSE
            AND ap.id != cp.id
    ) THEN
        RAISE EXCEPTION 'cannot finalize chat provider migration because a live agents-* AI provider name already exists';
    END IF;
END $$;

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
FROM chat_providers cp
WHERE NOT EXISTS (
    SELECT 1
    FROM ai_providers ap
    WHERE ap.id = cp.id
);

UPDATE ai_providers ap
SET
    type = cp.provider::ai_provider_type,
    name = 'agents-' || cp.provider,
    display_name = NULLIF(cp.display_name, ''),
    enabled = cp.enabled,
    deleted = FALSE,
    base_url = cp.base_url,
    updated_at = GREATEST(cp.updated_at, ap.updated_at)
FROM chat_providers cp
WHERE ap.id = cp.id
    AND (cp.updated_at > ap.updated_at OR ap.deleted);

DELETE FROM ai_provider_keys apk
USING chat_providers cp
WHERE cp.id = apk.provider_id
    AND cp.api_key = ''
    AND cp.updated_at > apk.updated_at;

WITH runtime_provider_keys AS (
    SELECT DISTINCT ON (apk.provider_id)
        apk.id,
        apk.provider_id
    FROM ai_provider_keys apk
    JOIN chat_providers cp ON cp.id = apk.provider_id
    WHERE cp.api_key != ''
    ORDER BY
        apk.provider_id ASC,
        apk.created_at ASC,
        apk.id ASC
)
UPDATE ai_provider_keys apk
SET
    api_key = cp.api_key,
    api_key_key_id = cp.api_key_key_id,
    updated_at = cp.updated_at
FROM runtime_provider_keys rpk
JOIN chat_providers cp ON cp.id = rpk.provider_id
WHERE apk.id = rpk.id
    AND cp.updated_at > apk.updated_at;

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
    cp.updated_at,
    cp.updated_at
FROM chat_providers cp
WHERE cp.api_key != ''
    AND NOT EXISTS (
        SELECT 1
        FROM ai_provider_keys apk
        WHERE apk.provider_id = cp.id
    );

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
FROM user_chat_provider_keys ucpk
ON CONFLICT (user_id, ai_provider_id) DO UPDATE
SET
    api_key = EXCLUDED.api_key,
    api_key_key_id = EXCLUDED.api_key_key_id,
    updated_at = EXCLUDED.updated_at
WHERE user_ai_provider_keys.updated_at < EXCLUDED.updated_at;

UPDATE chat_model_configs cmc
SET ai_provider_id = cp.id
FROM chat_providers cp
WHERE cmc.provider = cp.provider
    AND cmc.ai_provider_id IS NULL;

ALTER TABLE chat_model_configs
    ADD CONSTRAINT chat_model_configs_ai_provider_required_when_active
    CHECK (deleted = TRUE OR ai_provider_id IS NOT NULL);

DROP TABLE IF EXISTS user_chat_provider_keys;
DROP TABLE IF EXISTS chat_providers;
