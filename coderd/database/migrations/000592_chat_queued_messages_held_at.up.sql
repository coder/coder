ALTER TABLE chat_queued_messages ADD COLUMN held_at timestamptz;

COMMENT ON COLUMN chat_queued_messages.held_at IS 'Set while the owner is editing the row. The state machine treats the first held row and everything behind it as absent from the queue.';

-- Recreate the update trigger so hold changes advance queue_version and
-- reach open streams as a queue_update event.
DROP TRIGGER trigger_bump_chat_queue_version_on_queued_message_update ON chat_queued_messages;

CREATE TRIGGER trigger_bump_chat_queue_version_on_queued_message_update
AFTER UPDATE OF content, model_config_id, position, created_by, held_at
ON chat_queued_messages
FOR EACH ROW
EXECUTE FUNCTION bump_chat_queue_version_on_queued_message_change();
