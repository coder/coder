-- The old type cannot hold skipped or canceled, so the cast below fails on
-- any row that still has them. exit_failure with the runner's "unknown"
-- exit code is the nearest older meaning, and an older dashboard also
-- throws on a status it has no legend for.
UPDATE workspace_agent_script_timings
SET status = 'exit_failure', exit_code = 255
WHERE status IN ('skipped', 'canceled');

CREATE TYPE old_workspace_agent_script_timing_status AS ENUM (
	'ok',
	'exit_failure',
	'timed_out',
	'pipes_left_open'
);

ALTER TABLE workspace_agent_script_timings
	ALTER COLUMN status TYPE old_workspace_agent_script_timing_status
	USING (status::text::old_workspace_agent_script_timing_status);

DROP TYPE workspace_agent_script_timing_status;
ALTER TYPE old_workspace_agent_script_timing_status RENAME TO workspace_agent_script_timing_status;

COMMENT ON TYPE workspace_agent_script_timing_status IS 'What the exit status of the script is.';
