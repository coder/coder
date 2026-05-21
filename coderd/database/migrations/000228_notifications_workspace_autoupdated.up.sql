INSERT INTO notification_templates (id, name, title_template, body_template, "group", actions)
VALUES ('c34a0c09-0704-4cac-bd1c-0c0146811c2b', 'Workspace updated automatically', E'Workspace "{{.Labels.name}}" updated automatically',
        E'Hi {{.UserName}}\n\Your workspace **{{.Labels.name}}** has been updated automatically to the latest template version ({{.Labels.template_version_name}}).',
        'Workspace Events', '[
        {
            "label": "View workspace",
            "url": "{{ base_url }}/@{{.UserName}}/{{.Labels.name}}"
        }
    ]'::jsonb);
