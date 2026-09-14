-- Re-add `paused` to the chat_status enum (removed by 000543).
ALTER TYPE chat_status ADD VALUE IF NOT EXISTS 'paused';

ALTER TABLE chat_queued_messages ADD COLUMN held_at timestamptz;

COMMENT ON COLUMN chat_queued_messages.held_at IS 'Set while the owner edits the row. The state machine does not promote a held head.';

-- At most one held row per chat.
CREATE UNIQUE INDEX chat_queued_messages_one_held_per_chat
ON chat_queued_messages (chat_id)
WHERE held_at IS NOT NULL;

-- Hold changes bump queue_version so they reach open streams.
DROP TRIGGER trigger_bump_chat_queue_version_on_queued_message_update ON chat_queued_messages;

CREATE TRIGGER trigger_bump_chat_queue_version_on_queued_message_update
AFTER UPDATE OF content, model_config_id, position, created_by, held_at
ON chat_queued_messages
FOR EACH ROW
EXECUTE FUNCTION bump_chat_queue_version_on_queued_message_change();
