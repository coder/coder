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

UPDATE chat_model_configs cmc
SET ai_provider_id = cp.id
FROM chat_providers cp
WHERE cmc.provider = cp.provider
    AND cmc.ai_provider_id IS NULL;
