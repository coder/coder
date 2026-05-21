-- Task transition to 'complete' status
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
			 '8c5a4d12-9f7e-4b3a-a1c8-6e4f2d9b5a7c',
			 'Task Completed',
			 E'Task ''{{.Labels.workspace}}'' completed',
			 E'The task ''{{.Labels.task}}'' has completed successfully.',
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

-- Task transition to 'failed' status
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
			 '3b7e8f1a-4c2d-49a6-b5e9-7f3a1c8d6b4e',
			 'Task Failed',
			 E'Task ''{{.Labels.workspace}}'' failed',
			 E'The task ''{{.Labels.task}}'' has failed. Check the logs for more details.',
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
