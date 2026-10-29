-- Serves chat history pages, which read one chat's messages in id order. Without
-- it the planner can walk chat_messages_pkey through every other chat's rows.
CREATE INDEX idx_chat_messages_chat_id
    ON chat_messages (chat_id, id DESC)
    WHERE deleted = false;
