DROP INDEX IF EXISTS idx_chat_messages_chat_user_anchor;
ALTER TABLE chat_messages DROP COLUMN tool_call_count;
