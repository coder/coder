ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_project:share';

ALTER TABLE chat_projects
    ADD COLUMN user_acl jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN group_acl jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD CONSTRAINT chat_projects_user_acl_is_object CHECK (jsonb_typeof(user_acl) = 'object'),
    ADD CONSTRAINT chat_projects_group_acl_is_object CHECK (jsonb_typeof(group_acl) = 'object');

COMMENT ON COLUMN chat_projects.user_acl IS 'Per-user permissions granted on the project, keyed by user ID. Same shape as chats.user_acl.';
COMMENT ON COLUMN chat_projects.group_acl IS 'Per-group permissions granted on the project, keyed by group ID. Same shape as chats.group_acl.';

INSERT INTO notification_templates (
    id,
    name,
    title_template,
    body_template,
    actions,
    "group",
    method,
    kind,
    enabled_by_default
)
VALUES (
    '3f1c6d8a-5b2e-4f7a-9c0d-7e8b2a4c6d1f',
    'Chat Project Shared',
    E'{{.Labels.initiator}} shared a project with you',
    E'{{.Labels.initiator}} shared the project "**{{.Labels.project_name}}**" with you.',
    '[
        {
            "label": "Open project",
            "url": "{{base_url}}/agents/projects/{{.Labels.project_id}}"
        }
    ]'::jsonb,
    'Chat Events',
    NULL,
    'system'::notification_template_kind,
    true
);
