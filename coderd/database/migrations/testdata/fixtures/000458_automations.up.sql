INSERT INTO chat_automations (
    id,
    owner_id,
    organization_id,
    name,
    description,
    instructions,
    model_config_id,
    mcp_server_ids,
    allowed_tools,
    status,
    max_chat_creates_per_hour,
    max_messages_per_hour,
    created_at,
    updated_at
)
SELECT
    'b3d0fd0e-8e1a-4f2c-9a3b-1234567890ab',
    u.id,
    o.id,
    'fixture-automation',
    'Fixture automation for migration testing.',
    'You are a helpful assistant.',
    NULL,
    '{}',
    '{}',
    'active',
    10,
    60,
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
FROM users u
CROSS JOIN organizations o
ORDER BY u.created_at, u.id
LIMIT 1;

INSERT INTO chat_automation_triggers (
    id,
    automation_id,
    type,
    webhook_secret,
    webhook_secret_key_id,
    cron_schedule,
    last_triggered_at,
    filter,
    label_paths,
    created_at,
    updated_at
) VALUES (
    'c4e1fe1f-9f2b-4a3d-ab4c-234567890abc',
    'b3d0fd0e-8e1a-4f2c-9a3b-1234567890ab',
    'webhook',
    'whsec_fixture_secret',
    NULL,
    NULL,
    NULL,
    '{"action": "opened"}'::jsonb,
    '{"repo": "repository.full_name"}'::jsonb,
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
);

INSERT INTO chat_automation_events (
    id,
    automation_id,
    trigger_id,
    received_at,
    payload,
    filter_matched,
    resolved_labels,
    matched_chat_id,
    created_chat_id,
    status,
    error
) VALUES (
    'd5f20f20-a03c-4b4e-bc5d-345678901bcd',
    'b3d0fd0e-8e1a-4f2c-9a3b-1234567890ab',
    'c4e1fe1f-9f2b-4a3d-ab4c-234567890abc',
    '2024-01-01 00:00:00+00',
    '{"action": "opened", "repository": {"full_name": "coder/coder"}}'::jsonb,
    TRUE,
    '{"repo": "coder/coder"}'::jsonb,
    NULL,
    NULL,
    'preview',
    NULL
);
