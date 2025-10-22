INSERT INTO notification_templates (id, name, title_template, body_template, "group", actions)
VALUES ('f44d9314-ad03-4bc8-95d0-5cad491da6b6', 'User account deleted', E'User account "{{.Labels.deleted_account_name}}" deleted',
        E'Hi {{.UserName}},\n\nUser account **{{.Labels.deleted_account_name}}** has been deleted.',
        'User Events', '[
        {
            "label": "View accounts",
            "url": "{{ base_url }}/deployment/users?filter=status%3Aactive"
        }
    ]'::jsonb);
