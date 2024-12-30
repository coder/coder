INSERT INTO notification_templates
	(id, name, title_template, body_template, "group", actions)
VALUES (
	'd089fe7b-d5c5-4c0c-aaf5-689859f7d392',
	'Workspace Manually Updated',
	E'Workspace ''{{.Labels.workspace}}'' has been manually updated',
	E'Hello {{.UserName}},\n\n'||
		E'Your workspace **{{.Labels.workspace}}** has been manually updated to template version **{{.Labels.version}}**.',
    'Workspace Events',
	'[
		{
			"label": "See workspace",
			"url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"
		},
		{
			"label": "See template version",
			"url": "{{base_url}}/templates/{{.Labels.organization}}/{{.Labels.template}}/versions/{{.Labels.version}}"
		}
	]'::jsonb
);
