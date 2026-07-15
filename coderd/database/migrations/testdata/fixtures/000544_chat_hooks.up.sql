-- Fixture coverage for the chat_hook_dispatches table. Earlier chat
-- fixtures already insert at least one row into chats; attach a hook
-- dispatch to the first such chat without hard-coding a chat ID.
INSERT INTO chat_hook_dispatches (
    id,
    chat_id,
    event,
    turn_id,
    owner_id,
    started_at,
    finished_at,
    result,
    http_status,
    decision
)
SELECT
    '10000000-0000-0000-0000-00000000d15a'::uuid,
    chats.id,
    'pre_tool_use',
    '10000000-0000-0000-0000-00000000722d'::uuid,
    chats.owner_id,
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:01+00',
    'ok',
    200,
    'allow'
FROM chats
ORDER BY created_at, id
LIMIT 1;
