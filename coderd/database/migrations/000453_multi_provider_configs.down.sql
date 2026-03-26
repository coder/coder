-- Drop the partial unique index.
DROP INDEX IF EXISTS idx_chat_providers_one_enabled_per_family;

-- Restore the original unique constraint.
-- NOTE: This will fail if duplicate provider rows exist. That is
-- intentional — the rollback should not silently delete data.
ALTER TABLE chat_providers ADD CONSTRAINT chat_providers_provider_key UNIQUE (provider);

-- Restore the FK from chat_model_configs.provider → chat_providers(provider).
ALTER TABLE chat_model_configs ADD CONSTRAINT chat_model_configs_provider_fkey
    FOREIGN KEY (provider) REFERENCES chat_providers(provider) ON DELETE CASCADE;
