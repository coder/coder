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
    'b5db9597-de2a-4dea-87e9-25cee6906b86',
    'AI Budget Warning Threshold Reached',
    E'You''re approaching your monthly AI budget limit',
    E'You have used more than {{.Labels.threshold}}% of your monthly AI budget ({{.Labels.limit}}). Effective group: **{{.Labels.group_name}}**.',
    '[]'::jsonb,
    'AI Budget',
    NULL,
    'system'::notification_template_kind,
    true
);
