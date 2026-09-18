-- 'paused': a turn finished at a queued message under edit. IF NOT EXISTS tolerates databases where the value survived earlier enum rewrites.
ALTER TYPE chat_status ADD VALUE IF NOT EXISTS 'paused';

ALTER TABLE chat_queued_messages ADD COLUMN editing_since timestamptz;

COMMENT ON COLUMN chat_queued_messages.editing_since IS 'Set while the owner edits the row. A row under edit is not promoted into history until the edit ends.';

-- At most one row under edit per chat.
CREATE UNIQUE INDEX chat_queued_messages_one_editing_per_chat
ON chat_queued_messages (chat_id)
WHERE editing_since IS NOT NULL;

-- Edit-marker changes bump queue_version so they reach open streams.
DROP TRIGGER trigger_bump_chat_queue_version_on_queued_message_update ON chat_queued_messages;

CREATE TRIGGER trigger_bump_chat_queue_version_on_queued_message_update
AFTER UPDATE OF content, model_config_id, position, created_by, editing_since
ON chat_queued_messages
FOR EACH ROW
EXECUTE FUNCTION bump_chat_queue_version_on_queued_message_change();
