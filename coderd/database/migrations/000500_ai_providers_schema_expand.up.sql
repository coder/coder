CREATE TABLE user_ai_provider_keys (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ai_provider_id uuid NOT NULL REFERENCES ai_providers(id) ON DELETE CASCADE,
    api_key        text NOT NULL CHECK (api_key != ''),
    api_key_key_id text REFERENCES dbcrypt_keys(active_key_digest),
    created_at     timestamp with time zone NOT NULL DEFAULT NOW(),
    updated_at     timestamp with time zone NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, ai_provider_id)
);

COMMENT ON TABLE user_ai_provider_keys IS 'User-owned API keys associated with AI providers. These keys are used only when BYOK is enabled.';

COMMENT ON COLUMN user_ai_provider_keys.api_key IS 'User-owned API key used to authenticate with the upstream AI provider. Encrypted at rest via dbcrypt when api_key_key_id is set.';

COMMENT ON COLUMN user_ai_provider_keys.api_key_key_id IS 'The ID of the key used to encrypt the user-owned provider API key. If this is NULL, the API key is not encrypted.';

CREATE INDEX idx_user_ai_provider_keys_ai_provider_id
    ON user_ai_provider_keys (ai_provider_id);

CREATE INDEX idx_user_ai_provider_keys_user_id
    ON user_ai_provider_keys (user_id);

ALTER TABLE chat_model_configs
    ADD COLUMN ai_provider_id uuid REFERENCES ai_providers(id);

CREATE INDEX idx_chat_model_configs_ai_provider_id
    ON chat_model_configs (ai_provider_id);
