DROP INDEX IF EXISTS idx_chat_model_configs_ai_provider_id;

ALTER TABLE chat_model_configs
    DROP COLUMN IF EXISTS ai_provider_id;

DROP INDEX IF EXISTS idx_user_ai_provider_keys_user_id;
DROP INDEX IF EXISTS idx_user_ai_provider_keys_ai_provider_id;
DROP TABLE IF EXISTS user_ai_provider_keys;
