INSERT INTO notification_templates (id, name, title_template, body_template, "group", actions)
VALUES ('b02ddd82-4733-4d02-a2d7-c36f3598997d', 'User account suspended', E'User account "{{.Labels.suspended_account_name}}" suspended',
        E'Hi {{.UserName}},\n\User account **{{.Labels.suspended_account_name}}** has been suspended.',
        'Workspace Events', '[
        {
            "label": "View accounts",
            "url": "{{ base_url }}/deployment/users?filter=status%3Aactive"
        }
    ]'::jsonb);
INSERT INTO notification_templates (id, name, title_template, body_template, "group", actions)
VALUES ('9f5af851-8408-4e73-a7a1-c6502ba46689', 'User account reactivated', E'User account "{{.Labels.reactivated_account_name}}" reactivated',
        E'Hi {{.UserName}},\n\User account **{{.Labels.reactivated_account_name}}** has been reactivated.',
        'Workspace Events', '[
        {
            "label": "View accounts",
            "url": "{{ base_url }}/deployment/users?filter=status%3Aactive"
        }
    ]'::jsonb);
