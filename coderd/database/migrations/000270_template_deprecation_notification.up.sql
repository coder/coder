INSERT INTO notification_templates
	(id, name, title_template, body_template, "group", actions)
VALUES (
	'f40fae84-55a2-42cd-99fa-b41c1ca64894',
	'Template Deprecated',
	E'Template ''{{.Labels.template}}'' has been deprecated',
    E'Hello {{.UserName}},\n\n'||
		E'The template **{{.Labels.template}}** has been deprecated with the following message:\n\n' ||
		E'**{{.Labels.message}}**\n\n' ||
		E'New workspaces may not be created from this template. Existing workspaces will continue to function normally.',
    'Template Events',
	'[
		{
			"label": "See affected workspaces",
			"url": "{{base_url}}/workspaces?filter=owner%3Ame+template%3A{{.Labels.template}}"
		},
		{
			"label": "View template",
			"url": "{{base_url}}/templates/{{.Labels.organization}}/{{.Labels.template}}"
		}
	]'::jsonb
);
