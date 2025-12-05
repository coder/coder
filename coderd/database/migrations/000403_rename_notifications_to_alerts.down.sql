-- Revert alerts system back to notifications system.

-- First, drop all triggers that depend on functions we'll rename.
DROP TRIGGER IF EXISTS update_alert_message_dedupe_hash ON alert_messages;
DROP TRIGGER IF EXISTS inhibit_enqueue_if_disabled ON alert_messages;

-- Drop the functions.
DROP FUNCTION IF EXISTS compute_alert_message_dedupe_hash();
DROP FUNCTION IF EXISTS inhibit_enqueue_if_disabled();

-- Rename tables back.
ALTER TABLE alert_templates RENAME TO notification_templates;
ALTER TABLE alert_messages RENAME TO notification_messages;
ALTER TABLE alert_preferences RENAME TO notification_preferences;
ALTER TABLE alert_report_generator_logs RENAME TO notification_report_generator_logs;
ALTER TABLE inbox_alerts RENAME TO inbox_notifications;

-- Rename columns back.
ALTER TABLE notification_messages RENAME COLUMN alert_template_id TO notification_template_id;
ALTER TABLE notification_preferences RENAME COLUMN alert_template_id TO notification_template_id;
ALTER TABLE notification_report_generator_logs RENAME COLUMN alert_template_id TO notification_template_id;

-- Rename types back.
ALTER TYPE inbox_alert_read_status RENAME TO inbox_notification_read_status;
ALTER TYPE alert_message_status RENAME TO notification_message_status;
ALTER TYPE alert_method RENAME TO notification_method;
ALTER TYPE alert_template_kind RENAME TO notification_template_kind;

-- Rename constraints back.
ALTER TABLE notification_templates RENAME CONSTRAINT alert_templates_pkey TO notification_templates_pkey;
ALTER TABLE notification_templates RENAME CONSTRAINT alert_templates_name_key TO notification_templates_name_key;
ALTER TABLE notification_messages RENAME CONSTRAINT alert_messages_pkey TO notification_messages_pkey;
ALTER TABLE notification_preferences RENAME CONSTRAINT alert_preferences_pkey TO notification_preferences_pkey;
ALTER TABLE notification_report_generator_logs RENAME CONSTRAINT alert_report_generator_logs_pkey TO notification_report_generator_logs_pkey;
ALTER TABLE inbox_notifications RENAME CONSTRAINT inbox_alerts_pkey TO inbox_notifications_pkey;

-- Rename foreign key constraints back.
ALTER TABLE notification_messages RENAME CONSTRAINT alert_messages_alert_template_id_fkey TO notification_messages_notification_template_id_fkey;
ALTER TABLE notification_messages RENAME CONSTRAINT alert_messages_user_id_fkey TO notification_messages_user_id_fkey;
ALTER TABLE notification_preferences RENAME CONSTRAINT alert_preferences_alert_template_id_fkey TO notification_preferences_notification_template_id_fkey;
ALTER TABLE notification_preferences RENAME CONSTRAINT alert_preferences_user_id_fkey TO notification_preferences_user_id_fkey;
ALTER TABLE inbox_notifications RENAME CONSTRAINT inbox_alerts_template_id_fkey TO inbox_notifications_template_id_fkey;
ALTER TABLE inbox_notifications RENAME CONSTRAINT inbox_alerts_user_id_fkey TO inbox_notifications_user_id_fkey;

-- Rename indexes back.
ALTER INDEX idx_alert_messages_status RENAME TO idx_notification_messages_status;
ALTER INDEX alert_messages_dedupe_hash_idx RENAME TO notification_messages_dedupe_hash_idx;
ALTER INDEX idx_inbox_alerts_user_id_read_at RENAME TO idx_inbox_notifications_user_id_read_at;
ALTER INDEX idx_inbox_alerts_user_id_template_id_targets RENAME TO idx_inbox_notifications_user_id_template_id_targets;

-- Recreate the dedupe hash function with the old name.
CREATE FUNCTION compute_notification_message_dedupe_hash() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.dedupe_hash = MD5(
        COALESCE(NEW.user_id::text, '') ||
        COALESCE(NEW.notification_template_id::text, '') ||
        COALESCE(NEW.targets::text, '') ||
        DATE(NOW())::text
    );
    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION compute_notification_message_dedupe_hash() IS 'Computes a unique hash which will be used to prevent duplicate messages from being enqueued on the same day';

-- Recreate the inhibit function.
CREATE FUNCTION inhibit_enqueue_if_disabled() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
BEGIN
    IF EXISTS (
        SELECT 1 FROM notification_templates
        LEFT JOIN notification_preferences
            ON  notification_preferences.notification_template_id = notification_templates.id
            AND notification_preferences.user_id = NEW.user_id
        WHERE notification_templates.id = NEW.notification_template_id AND (
            notification_preferences.disabled = TRUE
            OR
            (notification_templates.enabled_by_default = FALSE AND notification_preferences.notification_template_id IS NULL)
        )
    ) THEN
        RAISE EXCEPTION 'cannot enqueue message: notification is not enabled';
    END IF;
    RETURN NEW;
END;
$$;

-- Recreate triggers with old names.
CREATE TRIGGER update_notification_message_dedupe_hash BEFORE INSERT OR UPDATE ON notification_messages FOR EACH ROW EXECUTE FUNCTION compute_notification_message_dedupe_hash();
CREATE TRIGGER inhibit_enqueue_if_disabled BEFORE INSERT ON notification_messages FOR EACH ROW EXECUTE FUNCTION inhibit_enqueue_if_disabled();

-- Rename the resource_type enum values back.
ALTER TYPE resource_type RENAME VALUE 'alert_template' TO 'notification_template';
ALTER TYPE resource_type RENAME VALUE 'alerts_settings' TO 'notifications_settings';
