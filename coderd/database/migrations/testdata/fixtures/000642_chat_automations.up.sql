INSERT INTO chat_automations (
    id,
    organization_id,
    owner_id,
    name,
    created_by_chat_id,
    kind,
    enabled,
    prompt,
    target_mode,
    target_chat_id,
    when_busy,
    webhook_auth,
    webhook_secret,
    webhook_secret_version,
    last_run_at,
    last_run_outcome,
    last_run_reason,
    last_run_chat_id
)
SELECT
    '6a1f0c3e-2b7d-4c8e-9f10-3d5e7a9b1c21',
    c.organization_id,
    c.owner_id,
    'fixture-webhook',
    c.id,
    'webhook',
    true,
    'Summarize the webhook payload.',
    'existing_chat',
    c.id,
    'queue',
    'bearer',
    'fixture-webhook-secret',
    1,
    '2026-01-05 09:00:00+00',
    'skipped',
    'chat_busy',
    c.id
FROM (
    SELECT id, organization_id, owner_id FROM chats ORDER BY created_at, id LIMIT 1
) AS c;

INSERT INTO chat_automations (
    id,
    organization_id,
    owner_id,
    name,
    kind,
    enabled,
    prompt,
    target_mode,
    new_chat_model_config_id,
    reasoning_effort,
    schedule_cron,
    schedule_time_zone,
    schedule_next_run_at
)
SELECT
    '7b2e1d4f-3c8e-4d9f-8a21-4e6f8b0c2d32',
    m.organization_id,
    u.id,
    'fixture-schedule',
    'schedule',
    true,
    'Post the daily status report.',
    'new_chat',
    m.id,
    'high',
    '0 9 * * 1-5',
    'UTC',
    '2026-01-05 09:00:00+00'
FROM (
    SELECT id, organization_id FROM chat_model_configs ORDER BY created_at, id LIMIT 1
) AS m
CROSS JOIN (
    SELECT id FROM users ORDER BY created_at, id LIMIT 1
) AS u;

INSERT INTO chat_queued_messages (
    chat_id,
    content,
    created_by,
    automation_id,
    input_id,
    prompt_hooks_pending
)
SELECT
    a.target_chat_id,
    '[{"type":"text","text":"Summarize the webhook payload."}]'::jsonb,
    a.owner_id,
    a.id,
    '8c3f2e5a-4d9f-4e0a-9b32-5f7a9c1d3e43',
    true
FROM chat_automations a
WHERE a.id = '6a1f0c3e-2b7d-4c8e-9f10-3d5e7a9b1c21';

UPDATE chat_messages
SET
    automation_id = '6a1f0c3e-2b7d-4c8e-9f10-3d5e7a9b1c21',
    input_id = '9d4a3f6b-5e0a-4f1b-8c43-6a8b0d2e4f54'
WHERE id = (
    SELECT id FROM chat_messages ORDER BY id LIMIT 1
);

UPDATE chats
SET automation_id = '7b2e1d4f-3c8e-4d9f-8a21-4e6f8b0c2d32'
WHERE id = (
    SELECT id FROM chats ORDER BY created_at, id LIMIT 1
);
