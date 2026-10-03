-- Take the chat_automations lock before changing anything, retrying short
-- waits so that concurrent chatd traffic cannot deadlock with it. See
-- 000609_chat_automations.up.sql for the reasoning.
DO $$
DECLARE
	previous_lock_timeout text := current_setting('lock_timeout');
	deadline timestamptz := clock_timestamp() + interval '2 minutes';
BEGIN
	LOOP
		BEGIN
			PERFORM set_config('lock_timeout', '100ms', true);
			LOCK TABLE chat_automations IN ACCESS EXCLUSIVE MODE;
			EXIT;
		EXCEPTION WHEN lock_not_available OR deadlock_detected THEN
			IF clock_timestamp() > deadline THEN
				RAISE EXCEPTION 'migration 000612 could not lock chat_automations within 2 minutes';
			END IF;
		END;
		PERFORM pg_sleep(0.1);
	END LOOP;
	PERFORM set_config('lock_timeout', previous_lock_timeout, true);
END;
$$;

DROP TRIGGER IF EXISTS trigger_clear_chat_automation_schedule_claim ON chat_automations;
DROP FUNCTION IF EXISTS clear_chat_automation_schedule_claim();

ALTER TABLE chat_automations
    DROP COLUMN schedule_claimed_until;
