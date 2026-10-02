-- Take the table lock before changing anything, retrying short waits. chatd
-- keeps using chat_automations during a rolling deploy: publishers and the
-- schedule scanner lock a chat and then its automations, while a management
-- update locks an automation and then reads the target chat through the
-- target_chat_id foreign key. A migration that waits for the full ACCESS
-- EXCLUSIVE lock can close a cycle between those two orders. Each attempt
-- gives up after a short wait and releases its place in the lock queue.
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

-- Each kind leaves the other kind's columns NULL. Existing rows are not
-- cleaned up: chatd never writes the other kind's columns, so a row that
-- violates this fails the migration instead of losing data silently.
ALTER TABLE chat_automations
    DROP CONSTRAINT chat_automations_kind_shape,
    ADD CONSTRAINT chat_automations_kind_shape CHECK (
        (kind = 'webhook' AND webhook_use IS NOT NULL AND schedule_cron IS NULL AND schedule_time_zone IS NULL AND schedule_next_run_at IS NULL)
        OR (kind = 'schedule' AND webhook_use IS NULL AND webhook_secret_hash IS NULL AND webhook_consumed_at IS NULL AND schedule_cron IS NOT NULL AND schedule_time_zone IS NOT NULL)
    );
