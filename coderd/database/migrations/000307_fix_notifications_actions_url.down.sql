UPDATE notification_templates
SET
	actions = '[
		{
			"label": "View workspace",
			"url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"
		}
	]'::jsonb
WHERE id = '281fdf73-c6d6-4cbb-8ff5-888baf8a2fff';

UPDATE notification_templates
SET
	actions = '[
		{
			"label": "View workspace",
			"url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"
		},
		{
			"label": "View template version",
			"url": "{{base_url}}/templates/{{.Labels.organization}}/{{.Labels.template}}/versions/{{.Labels.version}}"
		}
	]'::jsonb WHERE id = 'd089fe7b-d5c5-4c0c-aaf5-689859f7d392';
