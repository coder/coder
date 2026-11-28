INSERT INTO chat_automations (
    id, organization_id, owner_id, name, created_by_chat_id, kind, enabled, prompt,
    target_mode, target_chat_id
)
SELECT
    '8c3f2e5a-4d9f-4e1a-9b32-5f7a9c1d3e44', c.organization_id, c.owner_id, 'fixture-webhook', c.id,
    'mcp_event', true, 'Summarize the new event.', 'existing_chat', c.id
FROM (SELECT id, organization_id, owner_id FROM chats ORDER BY created_at, id LIMIT 1) AS c;

INSERT INTO chat_automation_mcp_subscriptions (
    automation_id, mcp_server_config_id, event_name, arguments, delivery, state, cursor,
    webhook_subscription_id, refresh_before, renew_at
)
SELECT
    '8c3f2e5a-4d9f-4e1a-9b32-5f7a9c1d3e44', m.id, 'ci.run.completed', '{"repo":"coder/coder"}',
    'webhook', 'active', 'seq:3', 'sub_a3f1c8e2b0d49f7e', '2026-01-05 10:30:00+00', '2026-01-05 10:15:00+00'
FROM (SELECT id FROM mcp_server_configs ORDER BY created_at, id LIMIT 1) AS m;

INSERT INTO chat_automation_webhook_secrets (automation_id, secret)
VALUES ('8c3f2e5a-4d9f-4e1a-9b32-5f7a9c1d3e44', 'whsec_MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=');
