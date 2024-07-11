INSERT INTO notification_templates (id, name, title_template, body_template, "group", actions)
VALUES ('123e4567-e89b-12d3-a456-426614174000', 'Workspace Marked as Dormant', E'Workspace "{{.Labels.name}}" marked as dormant',
        E'Hi {{.UserName}}\n\nYour workspace **{{.Labels.name}}** has been marked as dormant.\nThe specified reason was "**{{.Labels.reason}}{{ if .Labels.initiator }} ({{ .Labels.initiator }}){{end}}**',
        'Workspace Events', '[
        {
			"label": "View workspace",
			"url": "{{ base_url }}/@{{.UserName}}/{{.Labels.name}}"
		}
    ]'::jsonb);
