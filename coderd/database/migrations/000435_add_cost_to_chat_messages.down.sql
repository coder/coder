DROP INDEX IF EXISTS idx_chat_messages_created_at;

ALTER TABLE chat_messages DROP COLUMN total_cost_micros;
