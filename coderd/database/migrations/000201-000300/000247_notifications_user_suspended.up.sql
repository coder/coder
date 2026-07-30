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
VALUES ('6a2f0609-9b69-4d36-a989-9f5925b6cbff', 'Your account has been suspended', E'Your account "{{.Labels.suspended_account_name}}" has been suspended',
        E'Hi {{.UserName}},\nYour account **{{.Labels.suspended_account_name}}** has been suspended.',
        'User Events', '[]'::jsonb);
INSERT INTO notification_templates (id, name, title_template, body_template, "group", actions)
VALUES ('9f5af851-8408-4e73-a7a1-c6502ba46689', 'User account activated', E'User account "{{.Labels.activated_account_name}}" activated',
        E'Hi {{.UserName}},\nUser account **{{.Labels.activated_account_name}}** has been activated.',
        'User Events', '[
        {
            "label": "View accounts",
            "url": "{{ base_url }}/deployment/users?filter=status%3Aactive"
        }
    ]'::jsonb);
INSERT INTO notification_templates (id, name, title_template, body_template, "group", actions)
VALUES ('1a6a6bea-ee0a-43e2-9e7c-eabdb53730e4', 'Your account has been activated', E'Your account "{{.Labels.activated_account_name}}" has been activated',
        E'Hi {{.UserName}},\nYour account **{{.Labels.activated_account_name}}** has been activated.',
        'User Events', '[
        {
            "label": "Open Coder",
            "url": "{{ base_url }}"
        }
    ]'::jsonb);
