ALTER TABLE chat_providers
    ADD COLUMN central_api_key_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN allow_user_api_key BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN allow_central_api_key_fallback BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE chat_providers
    ADD CONSTRAINT valid_credential_policy CHECK (
        (central_api_key_enabled OR allow_user_api_key) AND
        (
            NOT allow_central_api_key_fallback OR
            (central_api_key_enabled AND allow_user_api_key)
        )
    );

CREATE TABLE user_chat_provider_keys (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chat_provider_id UUID        NOT NULL REFERENCES chat_providers(id) ON DELETE CASCADE,
    api_key          TEXT        NOT NULL CHECK (api_key != ''),
    api_key_key_id   TEXT        REFERENCES dbcrypt_keys(active_key_digest),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, chat_provider_id)
);
