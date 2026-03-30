-- Drop the unique constraint on chat_providers.provider so multiple
-- configs per provider family are allowed.
ALTER TABLE chat_providers DROP CONSTRAINT IF EXISTS chat_providers_provider_key;

-- The old FK (chat_model_configs.provider -> chat_providers.provider)
-- targeted the now-dropped unique column. Replace it with a proper
-- ID-based binding below.
ALTER TABLE chat_model_configs DROP CONSTRAINT IF EXISTS chat_model_configs_provider_fkey;

-- Bind each model config to a specific provider config by ID.
-- ON DELETE CASCADE removes bound models when the provider config is deleted,
-- preserving the same protection the old text-based FK provided.
ALTER TABLE chat_model_configs
    ADD COLUMN provider_config_id uuid REFERENCES chat_providers(id) ON DELETE CASCADE;
