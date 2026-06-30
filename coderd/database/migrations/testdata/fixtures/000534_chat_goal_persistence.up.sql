INSERT INTO chat_goals (
    id,
    root_chat_id,
    created_from_chat_id,
    objective,
    status,
    created_by_user_id,
    created_at,
    updated_at
)
SELECT
    'c8dcb6e1-85f6-48a3-8f70-2bc4e9b98025',
    '72c0438a-18eb-4688-ab80-e4c6a126ef96',
    '72c0438a-18eb-4688-ab80-e4c6a126ef96',
    'Fixture goal',
    'active',
    id,
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
FROM users
ORDER BY created_at, id
LIMIT 1;
