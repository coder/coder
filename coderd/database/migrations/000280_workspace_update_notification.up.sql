INSERT INTO notification_templates
	(id, name, title_template, body_template, "group", actions)
VALUES (
	'd089fe7b-d5c5-4c0c-aaf5-689859f7d392',
	'Workspace Manually Updated',
	E'Workspace ''{{.Labels.workspace}}'' has been manually updated',
	E'Hello {{.UserName}},\n\n'||
		E'A new workspace build has been manually created for your workspace **{{.Labels.workspace}}** by **{{.Labels.initiator}}** to update it to version **{{.Labels.version}}** of template **{{.Labels.template}}**.',
    'Workspace Events',
	'[
		{
			"label": "View workspace",
			"url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"
		},
		{
			"label": "View template version",
			"url": "{{base_url}}/templates/{{.Labels.organization}}/{{.Labels.template}}/versions/{{.Labels.version}}"
		}
	]'::jsonb
);

UPDATE notification_templates
SET
	actions = '[
		{
			"label": "View workspace",
			"url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"
		}
	]'::jsonb
WHERE id = '281fdf73-c6d6-4cbb-8ff5-888baf8a2fff';
