INSERT INTO notification_templates
	(id, name, title_template, body_template, "group", actions)
VALUES (
	'281fdf73-c6d6-4cbb-8ff5-888baf8a2fff',
	'Workspace Created',
	E'Workspace ''{{.Labels.workspace}}'' has been created',
    E'Hello {{.UserName}},\n\n'||
		E'The workspace **{{.Labels.workspace}}** has been created from the template **{{.Labels.template}}** using version **{{.Labels.version}}**.',
    'Workspace Events',
	'[
		{
			"label": "See workspace",
			"url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"
		}
	]'::jsonb
);
