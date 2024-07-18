INSERT INTO notification_templates (id, name, title_template, body_template, "group", actions)
VALUES ('123e4567-e89b-12d3-a456-426614174000', 'Workspace Marked as Dormant', E'Workspace "{{.Labels.name}}" marked as dormant',
        E'Hi {{.UserName}}\n\n' ||
        E'Your workspace **{{.Labels.name}}** has been marked as **dormant**.\n' ||
        E'The specified reason was "**{{.Labels.reason}}{{ if .Labels.initiator }} (initiated by: {{ .Labels.initiator }}){{end}}**\n\n' ||
        E'Dormancy refers to a workspace being unused for a defined length of time, and after it exceeds {{.Labels.dormancyHours}} hours of dormancy it will be deleted.\n' ||
        E'To prevent your workspace from being deleted, simply use it as normal.',
        'Workspace Events', '[
        {
			"label": "View workspace",
			"url": "{{ base_url }}/@{{.UserName}}/{{.Labels.name}}"
		}
    ]'::jsonb);
