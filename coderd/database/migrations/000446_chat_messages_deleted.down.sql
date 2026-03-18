DELETE FROM chat_messages WHERE deleted = true;
ALTER TABLE chat_messages DROP COLUMN deleted;
