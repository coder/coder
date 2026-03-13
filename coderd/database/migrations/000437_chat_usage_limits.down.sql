DROP INDEX IF EXISTS idx_chat_messages_owner_spend;
ALTER TABLE chat_messages DROP COLUMN IF EXISTS owner_id;
DROP TABLE IF EXISTS chat_usage_limit_overrides;
DROP TABLE IF EXISTS chat_usage_limit_config;
