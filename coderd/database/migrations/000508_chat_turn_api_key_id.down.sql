DROP INDEX IF EXISTS idx_chat_queued_messages_api_key_id;

DROP INDEX IF EXISTS idx_chat_messages_api_key_id;

ALTER TABLE chat_queued_messages
DROP COLUMN api_key_id;

ALTER TABLE chat_messages
DROP COLUMN api_key_id;
