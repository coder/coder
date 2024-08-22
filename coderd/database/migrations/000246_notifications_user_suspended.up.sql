INSERT INTO notification_templates (id, name, title_template, body_template, "group", actions)
VALUES ('b02ddd82-4733-4d02-a2d7-c36f3598997d', 'User account suspended', E'User account "{{.Labels.suspended_account_name}}" suspended',
        E'Hi {{.UserName}},\nUser account **{{.Labels.suspended_account_name}}** has been suspended.',
        'User Events', '[
        {
            "label": "View suspended accounts",
            "url": "{{ base_url }}/deployment/users?filter=status%3Asuspended"
        }
    ]'::jsonb);
INSERT INTO notification_templates (id, name, title_template, body_template, "group", actions)
VALUES ('9f5af851-8408-4e73-a7a1-c6502ba46689', 'User account activated', E'User account "{{.Labels.activated_account_name}}" activated',
        E'Hi {{.UserName}},\nUser account **{{.Labels.activated_account_name}}** has been activated.',
        'User Events', '[
        {
            "label": "View accounts",
            "url": "{{ base_url }}/deployment/users?filter=status%3Aactive"
        }
    ]'::jsonb);
