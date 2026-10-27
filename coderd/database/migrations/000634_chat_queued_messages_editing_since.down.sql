-- Lock both tables up front, as the up migration does. chatd locks a chat
-- row before its queued rows, and the statements below lock them in the
-- opposite order. Nothing below uses chats_expanded, so it is not locked.
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
				RAISE EXCEPTION 'migration 000634 down could not lock the chat tables within 2 minutes';
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

-- `paused` stays in the enum because dropping a value requires recreating
-- the type. Paused chats have queued rows, so they move to error: older
-- code accepts error with queued rows and continues on the next send, but
-- not waiting with queued rows.
UPDATE chats
SET status = 'error',
    last_error = jsonb_build_object(
        'message', 'Queued message editing was removed by a downgrade.',
        'kind', 'generic'
    )
WHERE status = 'paused';
