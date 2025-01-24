INSERT INTO notification_templates
	(id, name, title_template, body_template, "group", actions)
VALUES (
	'a9d027b4-ac49-4fb1-9f6d-45af15f64e7a',
	'Workspace Reached Resource Threshold',
	E'Workspace "{{.Labels.workspace}}" reached resource threshold',
	E'Hi {{.UserName}},\n\n'||
		E'Your workspace **{{.Labels.workspace}}** has reached the {{.Labels.threshold_type}} threshold set at **{{.Labels.threshold}}**.',
	'Workspace Events',
	'[
		{
			"label": "View workspace",
			"url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"
		}
	]'::jsonb
);
