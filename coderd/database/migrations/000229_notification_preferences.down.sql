DROP TABLE IF EXISTS notification_preferences;

ALTER TABLE notification_templates
    DROP COLUMN IF EXISTS method;

DROP TRIGGER IF EXISTS inhibit_enqueue_if_disabled_trigger ON notification_messages;
DROP FUNCTION IF EXISTS inhibit_enqueue_if_disabled;
