-- Retained execution runtime for the first fixture chat. Associate one of the
-- chat's existing messages so both sides of the new relationship are covered.
INSERT INTO chat_execution_steps (
    id,
    chat_id,
    history_version,
    generation_attempt,
    operation,
    outcome,
    runtime_ms,
    recorded_at
)
SELECT
    '583e0001-0000-4000-8000-000000000001'::uuid,
    chats.id,
    chats.history_version,
    chats.generation_attempt,
    'model',
    'completed',
    1234,
    '2024-01-01 00:00:00+00'
FROM chats
ORDER BY chats.created_at, chats.id
LIMIT 1;

UPDATE chat_messages
SET execution_step_id = '583e0001-0000-4000-8000-000000000001'::uuid
WHERE id = (
    SELECT chat_messages.id
    FROM chat_messages
    JOIN chat_execution_steps
        ON chat_execution_steps.chat_id = chat_messages.chat_id
    WHERE chat_execution_steps.id = '583e0001-0000-4000-8000-000000000001'::uuid
    ORDER BY chat_messages.id
    LIMIT 1
);
