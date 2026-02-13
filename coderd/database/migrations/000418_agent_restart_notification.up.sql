INSERT INTO notification_templates
	(id, name, title_template, body_template, "group", actions)
VALUES (
	'bb2bb51b-5d40-4e33-ae8b-f40f13bfcd24',
	'Workspace Agent Restarted',
	E'Your workspace agent "{{.Labels.agent}}" has been restarted',
	E'Your workspace **{{.Labels.workspace}}** agent **{{.Labels.agent}}** has been restarted **{{.Labels.restart_count}}** time(s) to recover from an unexpected exit ({{.Labels.reason}}: {{.Labels.kill_signal}}).',
	'Workspace Events',
	'[
		{
			"label": "View workspace",
			"url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"
		}
	]'::jsonb
);
