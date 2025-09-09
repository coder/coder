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
 	'39b1e189-c857-4b0c-877a-511144c18516',
	'Custom Notification',
	'{{.Labels.custom_title}}',
	'{{.Labels.custom_message}}',
    '[]',
    'Custom Events',
    NULL,
	'system'::notification_template_kind,
	true
);
