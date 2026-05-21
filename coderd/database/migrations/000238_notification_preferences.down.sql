ALTER TABLE notification_templates
    DROP COLUMN IF EXISTS method,
    DROP COLUMN IF EXISTS kind;

DROP TABLE IF EXISTS notification_preferences;
DROP TYPE IF EXISTS notification_template_kind;

DROP TRIGGER IF EXISTS inhibit_enqueue_if_disabled ON notification_messages;
DROP FUNCTION IF EXISTS inhibit_enqueue_if_disabled;
