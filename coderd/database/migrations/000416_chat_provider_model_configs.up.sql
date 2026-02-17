CREATE TABLE chat_providers (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    provider        TEXT        NOT NULL UNIQUE CHECK (provider IN ('openai', 'anthropic')),
    display_name    TEXT        NOT NULL DEFAULT '',
    api_key         TEXT        NOT NULL DEFAULT '',
    api_key_key_id  TEXT        REFERENCES dbcrypt_keys(active_key_digest),
    enabled         BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON COLUMN chat_providers.api_key_key_id IS 'The ID of the key used to encrypt the provider API key. If this is NULL, the API key is not encrypted';

CREATE INDEX idx_chat_providers_enabled ON chat_providers(enabled);

CREATE TABLE chat_model_configs (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    provider        TEXT        NOT NULL REFERENCES chat_providers(provider) ON DELETE CASCADE,
    model           TEXT        NOT NULL,
    display_name    TEXT        NOT NULL DEFAULT '',
    enabled         BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(provider, model)
);

CREATE INDEX idx_chat_model_configs_enabled ON chat_model_configs(enabled);
CREATE INDEX idx_chat_model_configs_provider ON chat_model_configs(provider);
