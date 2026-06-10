DROP INDEX IF EXISTS idx_chat_messages_content_tsv;
DROP TRIGGER IF EXISTS chat_messages_content_text ON chat_messages;
DROP FUNCTION IF EXISTS chat_messages_set_content_text();
ALTER TABLE chat_messages DROP COLUMN IF EXISTS content_text;
