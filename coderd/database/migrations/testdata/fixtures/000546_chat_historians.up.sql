-- Attach historian progress to an existing root chat so migration tests cover
-- the state table without depending on a specific chat fixture ID.
INSERT INTO chat_historian_states (
    root_chat_id,
    last_processed_history_version,
    created_at,
    updated_at
)
SELECT
    chats.id,
    0,
    '2026-07-21 00:00:00+00',
    '2026-07-21 00:00:00+00'
FROM chats
WHERE chats.parent_chat_id IS NULL
ORDER BY chats.created_at, chats.id
LIMIT 1;
