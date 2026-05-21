DROP TRIGGER IF EXISTS update_notification_message_dedupe_hash ON notification_messages;
DROP FUNCTION IF EXISTS compute_notification_message_dedupe_hash();
ALTER TABLE IF EXISTS notification_messages
    DROP COLUMN IF EXISTS dedupe_hash;