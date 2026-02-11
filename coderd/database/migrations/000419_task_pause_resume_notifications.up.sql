-- Task transition to 'paused' status
INSERT INTO notification_templates (
	id,
	name,
	title_template,
	body_template,
	actions,
	"group",
	method,
	kind,
	enabled_by_default
) VALUES (
			 '2a74f3d3-ab09-4123-a4a5-ca238f4f65a1',
			 'Task Paused',
			 E'Task ''{{.Labels.workspace}}'' is paused',
			 E'The task ''{{.Labels.task}}'' was paused ({{.Labels.pause_reason}}).',
			 '[
				 {
					 "label": "View task",
					 "url": "{{base_url}}/tasks/{{.UserUsername}}/{{.Labels.workspace}}"
				 },
				 {
					 "label": "View workspace",
					 "url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"
				 }
			 ]'::jsonb,
			 'Task Events',
			 NULL,
			 'system'::notification_template_kind,
			 true
		 );

-- Task transition to 'resumed' status
INSERT INTO notification_templates (
	id,
	name,
	title_template,
	body_template,
	actions,
	"group",
	method,
	kind,
	enabled_by_default
) VALUES (
			 '843ee9c3-a8fb-4846-afa9-977bec578649',
			 'Task Resumed',
			 E'Task ''{{.Labels.workspace}}'' has resumed',
			 E'The task ''{{.Labels.task}}'' has resumed.',
			 '[
				 {
					 "label": "View task",
					 "url": "{{base_url}}/tasks/{{.UserUsername}}/{{.Labels.workspace}}"
				 },
				 {
					 "label": "View workspace",
					 "url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"
				 }
			 ]'::jsonb,
			 'Task Events',
			 NULL,
			 'system'::notification_template_kind,
			 true
		 );
