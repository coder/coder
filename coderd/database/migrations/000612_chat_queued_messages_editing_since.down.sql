-- Take the locks this migration needs before changing anything, retrying
-- short waits. See 000609_chat_automations.up.sql for the reasoning. chatd
-- locks a chat row before its queued rows, while the statements below lock
-- chat_queued_messages before updating chats, so both tables are locked in
-- one attempt. chats_expanded can stay: nothing below locks it.
DO $$
DECLARE
	previous_lock_timeout text := current_setting('lock_timeout');
	deadline timestamptz := clock_timestamp() + interval '2 minutes';
BEGIN
	LOOP
		BEGIN
			PERFORM set_config('lock_timeout', '100ms', true);
			LOCK TABLE chats, chat_queued_messages IN ACCESS EXCLUSIVE MODE;
			EXIT;
		EXCEPTION WHEN lock_not_available OR deadlock_detected THEN
			IF clock_timestamp() > deadline THEN
				RAISE EXCEPTION 'migration 000612 down could not lock the chat tables within 2 minutes';
			END IF;
		END;
		PERFORM pg_sleep(0.1);
	END LOOP;
	PERFORM set_config('lock_timeout', previous_lock_timeout, true);
END;
$$;

DROP TRIGGER trigger_bump_chat_queue_version_on_queued_message_update ON chat_queued_messages;

CREATE TRIGGER trigger_bump_chat_queue_version_on_queued_message_update
AFTER UPDATE OF content, model_config_id, position, created_by
ON chat_queued_messages
FOR EACH ROW
EXECUTE FUNCTION bump_chat_queue_version_on_queued_message_change();

DROP INDEX chat_queued_messages_one_editing_per_chat;

ALTER TABLE chat_queued_messages DROP COLUMN editing_since;

-- `paused` stays in the enum. Dropping a value requires recreating the type.
-- A paused chat has queued rows. Waiting with queued rows is not a valid
-- state for the older code, error with queued rows is, and a send
-- continues from it.
UPDATE chats
SET status = 'error',
    last_error = jsonb_build_object(
        'message', 'Queued message editing was removed by a downgrade.',
        'kind', 'generic'
    )
WHERE status = 'paused';
