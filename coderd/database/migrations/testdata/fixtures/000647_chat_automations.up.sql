INSERT INTO chat_automations (
    id, organization_id, owner_id, name, created_by_chat_id, kind, enabled, prompt,
    target_mode, target_chat_id, schedule_cron, schedule_time_zone, schedule_next_run_at,
    last_run_at, last_run_outcome, last_run_reason, last_run_chat_id
)
SELECT
    '6a1f0c3e-2b7d-4c8e-9f10-3d5e7a9b1c21', c.organization_id, c.owner_id, 'fixture-heartbeat', c.id,
    'schedule', true, 'Check the build status.', 'existing_chat', c.id, '*/30 * * * *', 'Europe/Berlin',
    '2026-01-05 09:30:00+00', '2026-01-05 09:00:00+00', 'skipped', 'chat_busy', c.id
FROM (SELECT id, organization_id, owner_id FROM chats ORDER BY created_at, id LIMIT 1) AS c;

INSERT INTO chat_automations (
    id, organization_id, owner_id, name, kind, enabled, prompt, target_mode,
    new_chat_model_config_id, project_id, schedule_cron, schedule_time_zone, schedule_next_run_at
)
SELECT
    '7b2e1d4f-3c8e-4d9f-8a21-4e6f8b0c2d32', p.organization_id, p.owner_id, 'fixture-report', 'schedule',
    false, 'Post the daily status report.', 'new_chat', m.id, p.id, '0 9 * * 1-5', 'UTC', NULL
FROM chat_projects p
JOIN chat_model_configs m ON m.organization_id = p.organization_id
ORDER BY p.created_at, p.id, m.created_at, m.id
LIMIT 1;
