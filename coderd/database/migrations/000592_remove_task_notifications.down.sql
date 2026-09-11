-- Restore task notification templates. Inbox notification history deleted
-- by the cascade is not recoverable.
INSERT INTO notification_templates (
    id, name, title_template, body_template, actions, "group", method, kind, enabled_by_default
) VALUES (
    'bd4b7168-d05e-4e19-ad0f-3593b77aa90f',
    'Task Working',
    E'Task ''{{.Labels.workspace}}'' is working',
    E'The task ''{{.Labels.task}}'' transitioned to a working state.',
    '[{"label": "View task", "url": "{{base_url}}/tasks/{{.UserUsername}}/{{.Labels.workspace}}"}, {"label": "View workspace", "url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"}]'::jsonb,
    'Task Events',
    NULL,
    'system'::notification_template_kind,
    false
), (
    'd4a6271c-cced-4ed0-84ad-afd02a9c7799',
    'Task Idle',
    E'Task ''{{.Labels.workspace}}'' is idle',
    E'The task ''{{.Labels.task}}'' is idle and ready for input.',
    '[{"label": "View task", "url": "{{base_url}}/tasks/{{.UserUsername}}/{{.Labels.workspace}}"}, {"label": "View workspace", "url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"}]'::jsonb,
    'Task Events',
    NULL,
    'system'::notification_template_kind,
    false
), (
    '8c5a4d12-9f7e-4b3a-a1c8-6e4f2d9b5a7c',
    'Task Completed',
    E'Task ''{{.Labels.workspace}}'' completed',
    E'The task ''{{.Labels.task}}'' has completed successfully.',
    '[{"label": "View task", "url": "{{base_url}}/tasks/{{.UserUsername}}/{{.Labels.workspace}}"}, {"label": "View workspace", "url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"}]'::jsonb,
    'Task Events',
    NULL,
    'system'::notification_template_kind,
    false
), (
    '3b7e8f1a-4c2d-49a6-b5e9-7f3a1c8d6b4e',
    'Task Failed',
    E'Task ''{{.Labels.workspace}}'' failed',
    E'The task ''{{.Labels.task}}'' has failed. Check the logs for more details.',
    '[{"label": "View task", "url": "{{base_url}}/tasks/{{.UserUsername}}/{{.Labels.workspace}}"}, {"label": "View workspace", "url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"}]'::jsonb,
    'Task Events',
    NULL,
    'system'::notification_template_kind,
    false
), (
    '2a74f3d3-ab09-4123-a4a5-ca238f4f65a1',
    'Task Paused',
    E'Task ''{{.Labels.task}}'' is paused',
    E'The task ''{{.Labels.task}}'' was paused ({{.Labels.pause_reason}}).',
    '[{"label": "View task", "url": "{{base_url}}/tasks/{{.UserUsername}}/{{.Labels.task_id}}"}, {"label": "View workspace", "url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"}]'::jsonb,
    'Task Events',
    NULL,
    'system'::notification_template_kind,
    true
), (
    '843ee9c3-a8fb-4846-afa9-977bec578649',
    'Task Resumed',
    E'Task ''{{.Labels.task}}'' has resumed',
    E'The task ''{{.Labels.task}}'' has resumed.',
    '[{"label": "View task", "url": "{{base_url}}/tasks/{{.UserUsername}}/{{.Labels.task_id}}"}, {"label": "View workspace", "url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"}]'::jsonb,
    'Task Events',
    NULL,
    'system'::notification_template_kind,
    true
);
