INSERT INTO user_chat_provider_keys (
    user_id,
    chat_provider_id,
    api_key,
    created_at,
    updated_at
)
SELECT
    id,
    '0a8b2f84-b5a8-4c44-8c9f-e58c44a534a7',
    'fixture-test-key',
    '2025-01-01 00:00:00+00',
    '2025-01-01 00:00:00+00'
FROM users
ORDER BY created_at, id
LIMIT 1;
