-- Drop the FK from chat_model_configs.provider → chat_providers(provider).
ALTER TABLE chat_model_configs DROP CONSTRAINT IF EXISTS chat_model_configs_provider_fkey;

-- Drop the unique constraint on chat_providers.provider so multiple
-- configs per provider family are allowed.
ALTER TABLE chat_providers DROP CONSTRAINT IF EXISTS chat_providers_provider_key;

-- Enforce at most one enabled config per provider family.
CREATE UNIQUE INDEX idx_chat_providers_one_enabled_per_family
    ON chat_providers (provider)
    WHERE enabled = TRUE;
