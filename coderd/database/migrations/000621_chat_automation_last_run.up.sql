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
				RAISE EXCEPTION 'migration 000621 could not lock chat_automations within 2 minutes';
			END IF;
		END;
		PERFORM pg_sleep(0.1);
	END LOOP;
	PERFORM set_config('lock_timeout', previous_lock_timeout, true);
END;
$$;

CREATE TYPE chat_automation_run_outcome AS ENUM ('accepted', 'skipped', 'refused');

-- The outcome of the latest run only, not a run history. last_run_chat_id
-- has no foreign key, like the provenance columns: a deleted chat leaves
-- a dangling id, and readers resolve it as the caller.
ALTER TABLE chat_automations
    ADD COLUMN last_run_at timestamptz,
    ADD COLUMN last_run_outcome chat_automation_run_outcome,
    ADD COLUMN last_run_reason text,
    ADD COLUMN last_run_chat_id uuid,
    ADD CONSTRAINT chat_automations_last_run_shape CHECK (
        (last_run_at IS NULL) = (last_run_outcome IS NULL)
        AND (last_run_at IS NOT NULL OR (last_run_reason IS NULL AND last_run_chat_id IS NULL))
    );

COMMENT ON COLUMN chat_automations.last_run_at IS 'When the latest schedule occurrence, Run now, or authenticated webhook delivery was decided. NULL until the first one.';
COMMENT ON COLUMN chat_automations.last_run_outcome IS 'Outcome of the latest run: accepted, skipped (the target was busy or full, or the occurrence was missed), or refused.';
COMMENT ON COLUMN chat_automations.last_run_reason IS 'Short machine reason for a skipped or refused run, such as chat_busy or owner_inactive. NULL for accepted runs.';
COMMENT ON COLUMN chat_automations.last_run_chat_id IS 'Chat the latest run went to or was meant for. No foreign key by design.';
