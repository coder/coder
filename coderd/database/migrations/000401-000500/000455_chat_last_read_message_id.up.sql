ALTER TABLE chats ADD COLUMN last_read_message_id BIGINT;

-- Backfill existing chats so they don't appear unread after deploy.
-- The has_unread query uses COALESCE(last_read_message_id, 0), so
-- leaving this NULL would mark every existing chat as unread.
UPDATE chats SET last_read_message_id = (
    SELECT MAX(cm.id) FROM chat_messages cm
    WHERE cm.chat_id = chats.id AND cm.role = 'assistant' AND cm.deleted = false
);
