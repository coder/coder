-- Task transition to 'working' status
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
			 'bd4b7168-d05e-4e19-ad0f-3593b77aa90f',
			 'Task Working',
			 E'Task ''{{.Labels.workspace}}'' is working',
			 E'The task ''{{.Labels.task}}'' transitioned to a working state.',
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

-- Task transition to 'idle' status
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
			 'd4a6271c-cced-4ed0-84ad-afd02a9c7799',
			 'Task Idle',
			 E'Task ''{{.Labels.workspace}}'' is idle',
			 E'The task ''{{.Labels.task}}'' is idle and ready for input.',
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
