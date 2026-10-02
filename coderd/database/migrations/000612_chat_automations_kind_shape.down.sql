-- Take the table lock before changing anything, retrying short waits so that
-- concurrent chatd traffic cannot deadlock with it. See
-- 000612_chat_automations_kind_shape.up.sql for the reasoning.
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

ALTER TABLE chat_automations
    DROP CONSTRAINT chat_automations_kind_shape,
    ADD CONSTRAINT chat_automations_kind_shape CHECK (
        (kind = 'webhook' AND webhook_use IS NOT NULL AND schedule_cron IS NULL)
        OR (kind = 'schedule' AND webhook_use IS NULL AND webhook_secret_hash IS NULL AND schedule_cron IS NOT NULL AND schedule_time_zone IS NOT NULL)
    );
