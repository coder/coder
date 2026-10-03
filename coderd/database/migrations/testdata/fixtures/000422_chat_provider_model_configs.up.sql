INSERT INTO chat_providers (
    id,
    provider,
    display_name,
    api_key,
    api_key_key_id,
    enabled,
    created_at,
    updated_at
) VALUES
    (
        '0a8b2f84-b5a8-4c44-8c9f-e58c44a534a7',
        'openai',
        'OpenAI',
        '',
        NULL,
        TRUE,
        '2024-01-01 00:00:00+00',
        '2024-01-01 00:00:00+00'
    ),
    (
        '4f0a9c2e-1d3b-4a5c-8e7f-6a9b8c7d6e02',
        'anthropic',
        'Anthropic (Reasoning Effort Fixture)',
        '',
        NULL,
        TRUE,
        '2024-01-01 00:00:00+00',
        '2024-01-01 00:00:00+00'
    ),
    (
        '4f0a9c2e-1d3b-4a5c-8e7f-6a9b8c7d6e03',
        'azure',
        'Azure OpenAI (Reasoning Effort Fixture)',
        '',
        NULL,
        TRUE,
        '2024-01-01 00:00:00+00',
        '2024-01-01 00:00:00+00'
    ),
    (
        '4f0a9c2e-1d3b-4a5c-8e7f-6a9b8c7d6e04',
        'bedrock',
        'Bedrock (Reasoning Effort Fixture)',
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
    options,
    created_at,
    updated_at
) VALUES
    (
        '9af5f8d5-6a57-4505-8a69-3d6c787b95fd',
        'openai',
        'gpt-5.2',
        'GPT 5.2',
        TRUE,
        200000,
        70,
        '{}'::jsonb,
        '2024-01-01 00:00:00+00',
        '2024-01-01 00:00:00+00'
    ),
    (
        '4f0a9c2e-1d3b-4a5c-8e7f-6a9b8c7d6f01',
        'openai',
        'gpt-5.1',
        'GPT-5.1 (Legacy Effort)',
        TRUE,
        200000,
        70,
        '{"provider_options": {"openai": {"reasoning_effort": " HIGH ", "reasoning_summary": "auto"}}}',
        '2024-01-01 00:00:00+00',
        '2024-01-01 00:00:00+00'
    ),
    (
        '4f0a9c2e-1d3b-4a5c-8e7f-6a9b8c7d6f02',
        'anthropic',
        'claude-opus-4-6',
        'Claude Opus (Legacy Effort)',
        TRUE,
        200000,
        70,
        '{"provider_options": {"anthropic": {"effort": "max", "send_reasoning": true}}}',
        '2024-01-01 00:00:00+00',
        '2024-01-01 00:00:00+00'
    ),
    (
        '4f0a9c2e-1d3b-4a5c-8e7f-6a9b8c7d6f05',
        'azure',
        'gpt-5.1-azure',
        'Azure GPT-5.1 (Legacy Effort)',
        TRUE,
        200000,
        70,
        '{"provider_options": {"azure": {"reasoning_effort": "low"}}}',
        '2024-01-01 00:00:00+00',
        '2024-01-01 00:00:00+00'
    ),
    (
        '4f0a9c2e-1d3b-4a5c-8e7f-6a9b8c7d6f06',
        'bedrock',
        'anthropic.claude-opus-4-6',
        'Bedrock Claude Opus (Legacy Effort)',
        TRUE,
        200000,
        70,
        '{"provider_options": {"bedrock": {"effort": "minimal"}}}',
        '2024-01-01 00:00:00+00',
        '2024-01-01 00:00:00+00'
    ),
    (
        '4f0a9c2e-1d3b-4a5c-8e7f-6a9b8c7d6f03',
        'openai',
        'gpt-5.1-empty-effort',
        'GPT-5.1 (Empty Legacy Effort)',
        TRUE,
        200000,
        70,
        '{"provider_options": {"openai": {"reasoning_effort": ""}}}',
        '2024-01-01 00:00:00+00',
        '2024-01-01 00:00:00+00'
    ),
    (
        '4f0a9c2e-1d3b-4a5c-8e7f-6a9b8c7d6f04',
        'openai',
        'gpt-5.1-invalid-effort',
        'GPT-5.1 (Invalid Legacy Effort)',
        TRUE,
        200000,
        70,
        '{"provider_options": {"openai": {"reasoning_effort": "extreme"}}}',
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
