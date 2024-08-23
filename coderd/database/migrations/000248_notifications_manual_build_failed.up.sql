INSERT INTO notification_templates (id, name, title_template, body_template, "group", actions)
VALUES ('2faeee0f-26cb-4e96-821c-85ccb9f71513', 'Workspace Manual Build Failed', E'Workspace "{{.Labels.name}}" manual build failed',
        E'Hi {{.UserName}},\nA manual build of the workspace **{{.Labels.name}}** using the template **{{.Labels.template_name}}** failed.\nThe workspace build was initiated by **{{.Labels.initiator}}**.',
        'Workspace Events', '[
        {
            "label": "View workspace",
            "url": "{{ base_url }}/@{{.Labels.workspace_owner_username}}/{{.Labels.name}}"
        }
    ]'::jsonb);
