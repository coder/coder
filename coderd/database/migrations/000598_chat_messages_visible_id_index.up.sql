-- Serves reads of one chat's user-visible messages ordered by id, such as
-- SearchChatMessages and GetChatMessagesByChatIDDescPaginated. Without it a
-- LIMIT on id DESC under the deleted and visibility predicates scans every
-- row of the chat.
CREATE INDEX idx_chat_messages_chat_visible_id
    ON chat_messages (chat_id, id DESC)
    WHERE deleted = false AND visibility IN ('user', 'both');
