CREATE UNIQUE INDEX idx_chat_providers_one_enabled_per_family
    ON chat_providers (provider)
    WHERE enabled = TRUE;

DROP TABLE IF EXISTS chat_model_provider_configs;
