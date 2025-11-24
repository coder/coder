-- Create new enum with 'custom' value
CREATE TYPE new_notification_template_kind AS ENUM (
    'system',
    'custom'
);

-- Update the notification_templates table to use new enum
ALTER TABLE notification_templates
	ALTER COLUMN kind DROP DEFAULT,
	ALTER COLUMN kind TYPE new_notification_template_kind USING (kind::text::new_notification_template_kind),
    ALTER COLUMN kind SET DEFAULT 'system'::new_notification_template_kind;

-- Drop old enum and rename new one
DROP TYPE notification_template_kind;
ALTER TYPE new_notification_template_kind RENAME TO notification_template_kind;

-- Insert new Custom Notification template with 'custom' kind
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
	'custom'::notification_template_kind,
	true
);
