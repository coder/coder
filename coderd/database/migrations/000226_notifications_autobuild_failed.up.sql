INSERT INTO notification_templates (id, name, title_template, body_template, "group", actions)
VALUES ('381df2a9-c0c0-4749-420f-80a9280c66f9', 'Workspace Autobuild Failed', E'Workspace "{{.Labels.name}}" autobuild failed',
        E'Hi {{.UserName}}\n\Automatic build of your workspace **{{.Labels.name}}** failed.\nThe specified reason was "**{{.Labels.reason}}**".',
        'Workspace Events', '[
        {
            "label": "View workspace",
            "url": "{{ base_url }}/@{{.UserName}}/{{.Labels.name}}"
        }
    ]'::jsonb);
