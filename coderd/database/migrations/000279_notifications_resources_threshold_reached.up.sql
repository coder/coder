INSERT INTO notification_templates (id, name, title_template, body_template, "group", actions)
VALUES ('f17b00d1-f561-4881-8ef6-3d3194a2a1ca', 'Template threshold reached', E'You reached the set threshold for "{{.Labels.resource}}".',
        E'Hi {{.UserName}},\nYour workspace {{.Labels.workspace_name}} reached the threshold you have set {{.Labels.resource_threshold}} for {{.Labels.resource}}.',
        'User Events', ::jsonb);
