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
			-- Well below the default deadlock_timeout of 1s, so this
			-- transaction gives up before PostgreSQL can choose an
			-- application transaction as the deadlock victim.
			PERFORM set_config('lock_timeout', '100ms', true);
			LOCK TABLE chat_queued_messages IN ACCESS EXCLUSIVE MODE;
			EXIT;
		EXCEPTION WHEN lock_not_available OR deadlock_detected THEN
			IF clock_timestamp() > deadline THEN
				RAISE EXCEPTION 'migration 000633 down could not lock chat_queued_messages within 2 minutes';
			END IF;
		END;
		PERFORM pg_sleep(0.1);
	END LOOP;
	PERFORM set_config('lock_timeout', previous_lock_timeout, true);
END;
$$;

ALTER TABLE chat_queued_messages DROP COLUMN busy_behavior;
DROP TYPE chat_busy_behavior;
