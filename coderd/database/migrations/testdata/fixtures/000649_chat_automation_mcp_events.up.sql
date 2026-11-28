INSERT INTO chat_automations (
    id, organization_id, owner_id, name, created_by_chat_id, kind, enabled, prompt,
    target_mode, target_chat_id
)
SELECT
    '8c3f2e5a-4d9f-4e1a-9b32-5f7a9c1d3e43', c.organization_id, c.owner_id, 'fixture-event', c.id,
    'mcp_event', true, 'Summarize the new event.', 'existing_chat', c.id
FROM (SELECT id, organization_id, owner_id FROM chats ORDER BY created_at, id LIMIT 1) AS c;

INSERT INTO chat_automation_mcp_subscriptions (
    automation_id, mcp_server_config_id, event_name, arguments, delivery, cursor, next_poll_at
)
SELECT
    '8c3f2e5a-4d9f-4e1a-9b32-5f7a9c1d3e43', m.id, 'ci.run.completed', '{"repo":"coder/coder"}',
    'poll', 'seq:3', '2026-01-05 09:30:30+00'
FROM (SELECT id FROM mcp_server_configs ORDER BY created_at, id LIMIT 1) AS m;

INSERT INTO chat_automation_event_inbox (automation_id, kind, event_id, event_name, occurred_at, data)
VALUES ('8c3f2e5a-4d9f-4e1a-9b32-5f7a9c1d3e43', 'event', 'evt_1', 'ci.run.completed', '2026-01-05 09:30:00+00', '{"conclusion":"success"}');
