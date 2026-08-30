INSERT INTO chat_goals (
    id,
    root_chat_id,
    objective,
    status,
    created_by_user_id,
    created_at,
    updated_at
)
SELECT
    'c8dcb6e1-85f6-48a3-8f70-2bc4e9b98025',
    '72c0438a-18eb-4688-ab80-e4c6a126ef96',
    'Fixture goal',
    'active',
    id,
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
FROM users
ORDER BY created_at, id
LIMIT 1;

-- A running chat keeps its active goal through the 000590 backfill,
-- which pauses goals only on waiting or errored chats.
INSERT INTO chats (
    id,
    owner_id,
    last_model_config_id,
    organization_id,
    title,
    status,
    created_at,
    updated_at
)
SELECT
    'a3f1c5d7-2b48-4a6e-9c31-58f0d2b7ae14',
    owner_id,
    last_model_config_id,
    organization_id,
    'Fixture running chat',
    'running',
    created_at,
    updated_at
FROM chats
WHERE id = '72c0438a-18eb-4688-ab80-e4c6a126ef96';

INSERT INTO chat_goals (
    id,
    root_chat_id,
    objective,
    status,
    created_by_user_id,
    created_at,
    updated_at
)
SELECT
    'e5b9d2c4-7f13-4a86-b5d2-90c3f7a1e628',
    'a3f1c5d7-2b48-4a6e-9c31-58f0d2b7ae14',
    'Fixture running goal',
    'active',
    created_by_user_id,
    created_at,
    updated_at
FROM chat_goals
WHERE id = 'c8dcb6e1-85f6-48a3-8f70-2bc4e9b98025';
