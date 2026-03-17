DROP INDEX IF EXISTS idx_chat_messages_owner_spend;
ALTER TABLE groups DROP COLUMN IF EXISTS chat_spend_limit_micros;
ALTER TABLE users DROP COLUMN IF EXISTS chat_spend_limit_micros;
DROP TABLE IF EXISTS chat_usage_limit_config;
