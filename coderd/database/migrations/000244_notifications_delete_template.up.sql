INSERT INTO
    notification_templates (
        id,
        name,
        title_template,
        body_template,
        "group",
        actions
    )
VALUES (
        '29a09665-2a4c-403f-9648-54301670e7be',
        'Template Deleted',
        E'Template "{{.Labels.name}}" deleted',
        E'Hi {{.UserName}}\n\nThe template **{{.Labels.name}}** was deleted by **{{ .Labels.initiator }}**.',
        'Template Events',
        '[
        {
            "label": "View templates",
            "url": "{{ base_url }}/templates"
        }
    ]'::jsonb
    );
