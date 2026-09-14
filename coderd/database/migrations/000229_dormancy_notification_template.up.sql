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
        '0ea69165-ec14-4314-91f1-69566ac3c5a0',
        'Workspace Marked as Dormant',
        E'Workspace "{{.Labels.name}}" marked as dormant',
        E'Hi {{.UserName}}\n\n' || E'Your workspace **{{.Labels.name}}** has been marked as **dormant**.\n' || E'The specified reason was "**{{.Labels.reason}} (initiated by: {{ .Labels.initiator }}){{end}}**\n\n' || E'Dormancy refers to a workspace being unused for a defined length of time, and after it exceeds {{.Labels.dormancyHours}} hours of dormancy might be deleted.\n' || E'To activate your workspace again, simply use it as normal.',
        'Workspace Events',
        '[
        {
			"label": "View workspace",
			"url": "{{ base_url }}/@{{.UserName}}/{{.Labels.name}}"
		}
    ]'::jsonb
    ),
    (
        '51ce2fdf-c9ca-4be1-8d70-628674f9bc42',
        'Workspace Marked for Deletion',
        E'Workspace "{{.Labels.name}}" marked for deletion',
        E'Hi {{.UserName}}\n\n' || E'Your workspace **{{.Labels.name}}** has been marked for **deletion** after {{.Labels.dormancyHours}} hours of dormancy.\n' || E'The specified reason was "**{{.Labels.reason}}{{end}}**\n\n' || E'Dormancy refers to a workspace being unused for a defined length of time, and after it exceeds {{.Labels.dormancyHours}} hours of dormancy it will be deleted.\n' || E'To prevent your workspace from being deleted, simply use it as normal.',
        'Workspace Events',
        '[
        {
			"label": "View workspace",
			"url": "{{ base_url }}/@{{.UserName}}/{{.Labels.name}}"
		}
    ]'::jsonb
    );
