-- Fixture coverage for the chat_heartbeats table introduced in
-- migration 000500. The earlier chat fixtures already insert at least
-- one row into chats; we attach a heartbeat for the first such chat so
-- migration tests see a non-empty chat_heartbeats table without
-- hard-coding a specific chat ID.
INSERT INTO chat_heartbeats (
    chat_id,
    runner_id,
    heartbeat_at
)
SELECT
    chats.id,
    '00000000-0000-0000-0000-0000000fea51'::uuid,
    '2024-01-01 00:00:00+00'
FROM chats
ORDER BY created_at, id
LIMIT 1;
