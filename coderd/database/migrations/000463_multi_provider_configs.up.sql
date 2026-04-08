-- The old FK (chat_model_configs.provider -> chat_providers.provider)
-- depends on the unique provider index. Drop it before removing that
-- uniqueness constraint, then replace it with a proper ID-based binding
-- below.
ALTER TABLE chat_model_configs DROP CONSTRAINT IF EXISTS chat_model_configs_provider_fkey;

-- Drop the unique constraint on chat_providers.provider so multiple
-- configs per provider family are allowed.
ALTER TABLE chat_providers DROP CONSTRAINT IF EXISTS chat_providers_provider_key;

-- Bind each model config to a specific provider config by ID.
-- Keep this as a plain UUID so soft-deleted model configs preserve their
-- historical binding after the provider row is hard-deleted.
ALTER TABLE chat_model_configs
    ADD COLUMN provider_config_id uuid;

-- Backfill existing live model configs with the oldest enabled provider
-- config for their family, matching the precedence rule used at runtime
-- by GetEnabledChatProviderByProvider (created_at ASC, id ASC).
UPDATE chat_model_configs cmc
SET provider_config_id = (
    SELECT cp.id
    FROM chat_providers cp
    WHERE cp.provider = cmc.provider
      AND cp.enabled = TRUE
    ORDER BY cp.created_at ASC, cp.id ASC
    LIMIT 1
)
WHERE cmc.provider_config_id IS NULL
  AND cmc.deleted = FALSE;

-- Lookup index for model config -> provider config joins.
CREATE INDEX idx_chat_model_configs_provider_config_id
    ON chat_model_configs (provider_config_id)
    WHERE provider_config_id IS NOT NULL;

-- Non-unique lookup index for provider family queries now that
-- the old UNIQUE constraint was dropped above.
CREATE INDEX idx_chat_providers_provider
    ON chat_providers (provider);
