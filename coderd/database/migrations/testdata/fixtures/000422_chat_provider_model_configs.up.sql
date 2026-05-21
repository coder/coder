INSERT INTO chat_providers (
    id,
    provider,
    display_name,
    api_key,
    api_key_key_id,
    enabled,
    created_at,
    updated_at
) VALUES (
    '0a8b2f84-b5a8-4c44-8c9f-e58c44a534a7',
    'openai',
    'OpenAI',
    '',
    NULL,
    TRUE,
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
);

INSERT INTO chat_model_configs (
    id,
    provider,
    model,
    display_name,
    enabled,
    context_limit,
    compression_threshold,
    created_at,
    updated_at
) VALUES (
    '9af5f8d5-6a57-4505-8a69-3d6c787b95fd',
    'openai',
    'gpt-5.2',
    'GPT 5.2',
    TRUE,
    200000,
    70,
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
);

INSERT INTO chats (
    id,
    owner_id,
    last_model_config_id,
    title,
    status,
    created_at,
    updated_at
)
SELECT
    '72c0438a-18eb-4688-ab80-e4c6a126ef96',
    id,
    '9af5f8d5-6a57-4505-8a69-3d6c787b95fd',
    'Fixture Chat',
    'completed',
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
FROM users
ORDER BY created_at, id
LIMIT 1;

INSERT INTO chat_messages (
    chat_id,
    created_at,
    role,
    content
) VALUES (
    '72c0438a-18eb-4688-ab80-e4c6a126ef96',
    '2024-01-01 00:00:00+00',
    'assistant',
    '{"type":"text","text":"fixture"}'::jsonb
);

INSERT INTO chat_diff_statuses (
    chat_id,
    url,
    pull_request_state,
    changes_requested,
    additions,
    deletions,
    changed_files,
    refreshed_at,
    stale_at,
    created_at,
    updated_at,
    git_branch,
    git_remote_origin
) VALUES (
    '72c0438a-18eb-4688-ab80-e4c6a126ef96',
    'https://example.com/pr/1',
    'open',
    FALSE,
    1,
    0,
    1,
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00',
    'main',
    'origin'
);

INSERT INTO chat_queued_messages (
    chat_id,
    content,
    created_at
) VALUES (
    '72c0438a-18eb-4688-ab80-e4c6a126ef96',
    '{"type":"text","text":"queued fixture"}'::jsonb,
    '2024-01-01 00:00:00+00'
);
