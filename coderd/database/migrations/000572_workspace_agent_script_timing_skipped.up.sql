ALTER TYPE workspace_agent_script_timing_status
	ADD VALUE IF NOT EXISTS 'skipped';

COMMENT ON TYPE workspace_agent_script_timing_status IS 'The terminal outcome of a script execution.';
