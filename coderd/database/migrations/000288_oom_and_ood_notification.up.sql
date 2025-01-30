INSERT INTO notification_templates
	(id, name, title_template, body_template, "group", actions)
VALUES (
	'a9d027b4-ac49-4fb1-9f6d-45af15f64e7a',
	'Workspace Out Of Memory',
	E'Your workspace "{{.Labels.workspace}}" is low on memory',
	E'Hi {{.UserName}},\n\n'||
		E'Your workspace **{{.Labels.workspace}}** has reached the memory usage threshold set at **{{.Labels.threshold}}**.',
	'Workspace Events',
	'[
		{
			"label": "View workspace",
			"url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"
		}
	]'::jsonb
);

INSERT INTO notification_templates
	(id, name, title_template, body_template, "group", actions)
VALUES (
	'f047f6a3-5713-40f7-85aa-0394cce9fa3a',
	'Workspace Out Of Disk',
	E'Your workspace "{{.Labels.workspace}}" is low on disk',
	E'Hi {{.UserName}},\n\n'||
		E'Your workspace **{{.Labels.workspace}}** has reached the usage threshold set at **{{.Labels.threshold}}** for volume `{{.Labels.volume}}`.',
	'Workspace Events',
	'[
		{
			"label": "View workspace",
			"url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"
		}
	]'::jsonb
);
