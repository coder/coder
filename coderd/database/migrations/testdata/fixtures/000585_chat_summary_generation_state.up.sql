INSERT INTO chat_summary_generations (
    chat_id,
    started_at
)
SELECT
    chats.id,
    '2024-01-01 00:00:00+00'
FROM chats
ORDER BY created_at, id
LIMIT 1;
