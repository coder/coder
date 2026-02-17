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
    created_at,
    updated_at
) VALUES (
    '9af5f8d5-6a57-4505-8a69-3d6c787b95fd',
    'openai',
    'gpt-5.2',
    'GPT 5.2',
    TRUE,
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
);
