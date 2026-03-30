-- Drop the FK from chat_model_configs.provider → chat_providers(provider).
ALTER TABLE chat_model_configs DROP CONSTRAINT IF EXISTS chat_model_configs_provider_fkey;

-- Drop the unique constraint on chat_providers.provider so multiple
-- configs per provider family are allowed.
ALTER TABLE chat_providers DROP CONSTRAINT IF EXISTS chat_providers_provider_key;

-- Add a nullable binding from a model config to a specific provider config.
-- ON DELETE SET NULL preserves the model if the bound provider is removed.
ALTER TABLE chat_model_configs
    ADD COLUMN provider_config_id uuid REFERENCES chat_providers(id) ON DELETE SET NULL;
