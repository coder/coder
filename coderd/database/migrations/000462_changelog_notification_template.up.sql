INSERT INTO notification_templates (
    id,
    name,
    title_template,
    body_template,
    "group",
    actions
) VALUES (
    'b02d53fd-477d-4a65-8d42-1b7e4b38f8c3',
    'Changelog',
    E'{{.Labels.version}}',
    E'{{.Labels.summary}}',
    'Changelog',
    '[{"label":"View changelog","url":"/changelog/{{.Labels.version}}"}]'::jsonb
);
