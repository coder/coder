INSERT INTO notification_templates (id, name, title_template, body_template, "group", actions)
VALUES ('4e19c0ac-94e1-4532-9515-d1801aa283b2', 'User account created', E'User account "{{.Labels.created_account_name}}" created',
        E'Hi {{.UserName}},\n\New user account **{{.Labels.created_account_name}}** has been created.',
        'Workspace Events', '[
        {
            "label": "View accounts",
            "url": "{{ base_url }}/deployment/users?filter=status%3Aactive"
        }
    ]'::jsonb);
