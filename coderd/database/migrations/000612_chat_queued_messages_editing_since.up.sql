-- Take the lock this migration needs before changing anything, retrying
-- short waits so that a waiting migration does not stall chatd readers of
-- chat_queued_messages. See 000609_chat_automations.up.sql for the
-- reasoning. No statement below locks chats.
DO $$
DECLARE
	previous_lock_timeout text := current_setting('lock_timeout');
	deadline timestamptz := clock_timestamp() + interval '2 minutes';
BEGIN
	LOOP
		BEGIN
			PERFORM set_config('lock_timeout', '100ms', true);
			LOCK TABLE chat_queued_messages IN ACCESS EXCLUSIVE MODE;
			EXIT;
		EXCEPTION WHEN lock_not_available OR deadlock_detected THEN
			IF clock_timestamp() > deadline THEN
				RAISE EXCEPTION 'migration 000612 could not lock chat_queued_messages within 2 minutes';
			END IF;
		END;
		PERFORM pg_sleep(0.1);
	END LOOP;
	PERFORM set_config('lock_timeout', previous_lock_timeout, true);
END;
$$;

-- 'paused': a turn finished at a queued message under edit.
ALTER TYPE chat_status ADD VALUE IF NOT EXISTS 'paused';

ALTER TABLE chat_queued_messages ADD COLUMN editing_since timestamptz;

COMMENT ON COLUMN chat_queued_messages.editing_since IS 'Set while the owner edits the row. A row under edit is not promoted into history until the edit ends.';

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
