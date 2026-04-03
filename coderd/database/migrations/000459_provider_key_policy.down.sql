DROP TABLE IF EXISTS user_chat_provider_keys;

ALTER TABLE chat_providers DROP CONSTRAINT IF EXISTS valid_credential_policy;

ALTER TABLE chat_providers
    DROP COLUMN IF EXISTS central_api_key_enabled,
    DROP COLUMN IF EXISTS allow_user_api_key,
    DROP COLUMN IF EXISTS allow_central_api_key_fallback;
