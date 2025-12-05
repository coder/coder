-- Rename notification system to alerts system.

-- First, drop all triggers that depend on functions we'll rename.
DROP TRIGGER IF EXISTS update_notification_message_dedupe_hash ON notification_messages;
DROP TRIGGER IF EXISTS inhibit_enqueue_if_disabled ON notification_messages;

-- Drop the functions.
DROP FUNCTION IF EXISTS compute_notification_message_dedupe_hash();
DROP FUNCTION IF EXISTS inhibit_enqueue_if_disabled();

-- Rename types.
ALTER TYPE inbox_notification_read_status RENAME TO inbox_alert_read_status;
ALTER TYPE notification_message_status RENAME TO alert_message_status;
ALTER TYPE notification_method RENAME TO alert_method;
ALTER TYPE notification_template_kind RENAME TO alert_template_kind;

-- Rename tables.
ALTER TABLE notification_templates RENAME TO alert_templates;
ALTER TABLE notification_messages RENAME TO alert_messages;
ALTER TABLE notification_preferences RENAME TO alert_preferences;
ALTER TABLE notification_report_generator_logs RENAME TO alert_report_generator_logs;
ALTER TABLE inbox_notifications RENAME TO inbox_alerts;

-- Rename columns that reference old names.
ALTER TABLE alert_messages RENAME COLUMN notification_template_id TO alert_template_id;
ALTER TABLE alert_preferences RENAME COLUMN notification_template_id TO alert_template_id;
ALTER TABLE alert_report_generator_logs RENAME COLUMN notification_template_id TO alert_template_id;

-- Rename constraints.
ALTER TABLE alert_templates RENAME CONSTRAINT notification_templates_pkey TO alert_templates_pkey;
ALTER TABLE alert_templates RENAME CONSTRAINT notification_templates_name_key TO alert_templates_name_key;
ALTER TABLE alert_messages RENAME CONSTRAINT notification_messages_pkey TO alert_messages_pkey;
ALTER TABLE alert_preferences RENAME CONSTRAINT notification_preferences_pkey TO alert_preferences_pkey;
ALTER TABLE alert_report_generator_logs RENAME CONSTRAINT notification_report_generator_logs_pkey TO alert_report_generator_logs_pkey;
ALTER TABLE inbox_alerts RENAME CONSTRAINT inbox_notifications_pkey TO inbox_alerts_pkey;

-- Rename foreign key constraints.
ALTER TABLE alert_messages RENAME CONSTRAINT notification_messages_notification_template_id_fkey TO alert_messages_alert_template_id_fkey;
ALTER TABLE alert_messages RENAME CONSTRAINT notification_messages_user_id_fkey TO alert_messages_user_id_fkey;
ALTER TABLE alert_preferences RENAME CONSTRAINT notification_preferences_notification_template_id_fkey TO alert_preferences_alert_template_id_fkey;
ALTER TABLE alert_preferences RENAME CONSTRAINT notification_preferences_user_id_fkey TO alert_preferences_user_id_fkey;
ALTER TABLE inbox_alerts RENAME CONSTRAINT inbox_notifications_template_id_fkey TO inbox_alerts_template_id_fkey;
ALTER TABLE inbox_alerts RENAME CONSTRAINT inbox_notifications_user_id_fkey TO inbox_alerts_user_id_fkey;

-- Rename indexes.
ALTER INDEX idx_notification_messages_status RENAME TO idx_alert_messages_status;
ALTER INDEX notification_messages_dedupe_hash_idx RENAME TO alert_messages_dedupe_hash_idx;
ALTER INDEX idx_inbox_notifications_user_id_read_at RENAME TO idx_inbox_alerts_user_id_read_at;
ALTER INDEX idx_inbox_notifications_user_id_template_id_targets RENAME TO idx_inbox_alerts_user_id_template_id_targets;

-- Recreate the dedupe hash function with the new name.
CREATE FUNCTION compute_alert_message_dedupe_hash() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.dedupe_hash = MD5(
        COALESCE(NEW.user_id::text, '') ||
        COALESCE(NEW.alert_template_id::text, '') ||
        COALESCE(NEW.targets::text, '') ||
        DATE(NOW())::text
    );
    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION compute_alert_message_dedupe_hash() IS 'Computes a unique hash which will be used to prevent duplicate messages from being enqueued on the same day';

-- Recreate the inhibit function.
CREATE FUNCTION inhibit_enqueue_if_disabled() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
BEGIN
    IF EXISTS (
        -- If this returns any rows, it means the alert is disabled either
        --  * the user has disabled this alert.
        --  * the alert template is disabled by default and hasn't
        --    been enabled by the user.
        SELECT 1 FROM alert_templates
        LEFT JOIN alert_preferences
            ON  alert_preferences.alert_template_id = alert_templates.id
            AND alert_preferences.user_id = NEW.user_id
        WHERE alert_templates.id = NEW.alert_template_id AND (
            alert_preferences.disabled = TRUE
            OR
            (alert_templates.enabled_by_default = FALSE AND alert_preferences.alert_template_id IS NULL)
        )
    ) THEN
        RAISE EXCEPTION 'cannot enqueue message: alert is not enabled';
    END IF;
    RETURN NEW;
END;
$$;

-- Recreate triggers with new names.
CREATE TRIGGER update_alert_message_dedupe_hash BEFORE INSERT OR UPDATE ON alert_messages FOR EACH ROW EXECUTE FUNCTION compute_alert_message_dedupe_hash();
CREATE TRIGGER inhibit_enqueue_if_disabled BEFORE INSERT ON alert_messages FOR EACH ROW EXECUTE FUNCTION inhibit_enqueue_if_disabled();

-- Rename the resource_type enum values by recreating the type.
-- PostgreSQL doesn't support renaming enum values directly, but we can use ALTER TYPE ... RENAME VALUE.
ALTER TYPE resource_type RENAME VALUE 'notification_template' TO 'alert_template';
ALTER TYPE resource_type RENAME VALUE 'notifications_settings' TO 'alerts_settings';
