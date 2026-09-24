-- Fixture coverage for the chat_last_turn_summaries table introduced in
-- migration 000600. Earlier chat fixtures already insert at least one row
-- into chats; we attach a summary for the first such chat so migration tests
-- see a non-empty chat_last_turn_summaries table without hard-coding a chat ID.
INSERT INTO chat_last_turn_summaries (
    chat_id,
    last_turn_summary,
    history_version
)
SELECT
    chats.id,
    'resolved the issue',
    chats.history_version
FROM chats
ORDER BY created_at, id
LIMIT 1;
