-- Remove Custom Notification template
DELETE FROM notification_templates WHERE id = '39b1e189-c857-4b0c-877a-511144c18516';

-- Recreate the old enum without 'custom'
CREATE TYPE old_notification_template_kind AS ENUM ('system');

-- Update notification_templates to use the old enum
ALTER TABLE notification_templates
	ALTER COLUMN kind DROP DEFAULT,
	ALTER COLUMN kind TYPE old_notification_template_kind USING (kind::text::old_notification_template_kind),
  	ALTER COLUMN kind SET DEFAULT 'system'::old_notification_template_kind;

-- Drop the current enum and restore the original name
DROP TYPE notification_template_kind;
ALTER TYPE old_notification_template_kind RENAME TO notification_template_kind;
