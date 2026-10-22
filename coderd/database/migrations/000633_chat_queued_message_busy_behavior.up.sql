-- Take the lock this migration needs before changing anything, retrying
-- short waits so that concurrent chatd traffic cannot deadlock with it. See
-- 000609_chat_automations.up.sql for the reasoning.
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
				RAISE EXCEPTION 'migration chat_queued_message_busy_behavior could not lock chat_queued_messages within 2 minutes';
			END IF;
		END;
		PERFORM pg_sleep(0.1);
	END LOOP;
	PERFORM set_config('lock_timeout', previous_lock_timeout, true);
END;
$$;

CREATE TYPE chat_busy_behavior AS ENUM ('queue', 'steer');

ALTER TABLE chat_queued_messages
    ADD COLUMN busy_behavior chat_busy_behavior NOT NULL DEFAULT 'queue';

COMMENT ON COLUMN chat_queued_messages.busy_behavior IS 'queue: delivered at turn end. steer: delivered before the next model call.';
