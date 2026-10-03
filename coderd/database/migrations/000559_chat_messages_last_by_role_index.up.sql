-- Serves GetLastChatMessageByRole. It orders by id, so the existing
-- (chat_id, created_at) index cannot supply the LIMIT 1 row in index order.
CREATE INDEX idx_chat_messages_chat_role_id
    ON chat_messages (chat_id, role, id DESC)
    WHERE deleted = false;
