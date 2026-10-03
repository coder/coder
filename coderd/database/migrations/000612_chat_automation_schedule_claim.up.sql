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

ALTER TABLE chat_automations
    ADD COLUMN schedule_claimed_until timestamptz;

COMMENT ON COLUMN chat_automations.schedule_claimed_until IS 'Lease on the occurrence at schedule_next_run_at: the replica that set it runs the prompt hooks and publishes that occurrence. NULL or a past time means unclaimed.';

-- A claim names the occurrence at the cursor it was taken on. Every write
-- that moves the cursor or changes the schedule revision drops it, in the
-- database rather than in each query, so a writer that does not know the
-- column (a replica that predates it, during a rolling upgrade) cannot
-- leave a claim attached to the next occurrence.
CREATE FUNCTION clear_chat_automation_schedule_claim() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	IF NEW.schedule_next_run_at IS DISTINCT FROM OLD.schedule_next_run_at
		OR NEW.schedule_revision IS DISTINCT FROM OLD.schedule_revision THEN
		NEW.schedule_claimed_until := NULL;
	END IF;
	RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_clear_chat_automation_schedule_claim
    BEFORE UPDATE OF schedule_next_run_at, schedule_revision ON chat_automations
    FOR EACH ROW EXECUTE FUNCTION clear_chat_automation_schedule_claim();
