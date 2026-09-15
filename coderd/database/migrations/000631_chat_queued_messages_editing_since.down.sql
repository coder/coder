DROP TRIGGER trigger_bump_chat_queue_version_on_queued_message_update ON chat_queued_messages;

CREATE TRIGGER trigger_bump_chat_queue_version_on_queued_message_update
AFTER UPDATE OF content, model_config_id, position, created_by
ON chat_queued_messages
FOR EACH ROW
EXECUTE FUNCTION bump_chat_queue_version_on_queued_message_change();

DROP INDEX chat_queued_messages_one_editing_per_chat;

ALTER TABLE chat_queued_messages DROP COLUMN editing_since;

-- `paused` stays in the enum. Dropping a value requires recreating the type.
UPDATE chats SET status = 'waiting' WHERE status = 'paused';
