INSERT INTO chat_projects (id, organization_id, owner_id, name, invokable_agent_ids, workspace_tools_enabled, allowed_template_ids, mcp_server_ids, external_auth_provider_ids, template_parameter_rules)
VALUES (
    '59400000-0000-4000-8000-000000000628',
    'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1',
    '0ed9befc-4911-4ccf-a8e2-559bf72daa94',
    'Fixture agent',
    '{59400000-0000-4000-8000-000000000001}',
    false,
    '{}',
    '{}',
    '{github}',
    '[]'
);

INSERT INTO chat_project_triggers (id, project_id, organization_id, name, prompt, hashed_secret, created_by)
VALUES (
    '59400000-0000-4000-8000-000000000629',
    '59400000-0000-4000-8000-000000000628',
    'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1',
    'Fixture trigger',
    'Review the payload.',
    '\x00',
    '0ed9befc-4911-4ccf-a8e2-559bf72daa94'
);

INSERT INTO chat_project_triggers (id, project_id, organization_id, name, prompt, hashed_secret, created_by, source, schedule, next_run_at)
VALUES (
    '59400000-0000-4000-8000-000000000631',
    '59400000-0000-4000-8000-000000000628',
    'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1',
    'Fixture schedule trigger',
    'Summarize open issues.',
    '\x00',
    '0ed9befc-4911-4ccf-a8e2-559bf72daa94',
    'schedule',
    'CRON_TZ=UTC 0 9 * * 1-5',
    '2024-01-01 09:00:00+00'
);

INSERT INTO chat_project_trigger_deliveries (id, trigger_id, status, event, message)
VALUES (
    '59400000-0000-4000-8000-000000000630',
    '59400000-0000-4000-8000-000000000629',
    'ignored',
    'issues.opened',
    'The event came from issues, but the trigger only listens to pull_request.'
);

INSERT INTO chat_project_trigger_conversations (trigger_id, conversation_key, chat_id)
VALUES (
    '59400000-0000-4000-8000-000000000629',
    'T0123/C0123/1700000000.000100',
    'f5610000-0000-4000-8000-000000000007'
);

INSERT INTO chat_project_trigger_events (trigger_id, event_key)
VALUES (
    '59400000-0000-4000-8000-000000000629',
    'event:Ev0123'
);

INSERT INTO mcp_server_github_app_credentials (mcp_server_config_id, user_id, app_id, installation_id, private_key, repositories)
VALUES (
    'f5610000-0000-4000-8000-000000000005',
    '0ed9befc-4911-4ccf-a8e2-559bf72daa94',
    1234,
    5678,
    'fixture-private-key',
    '{coder}'
);
