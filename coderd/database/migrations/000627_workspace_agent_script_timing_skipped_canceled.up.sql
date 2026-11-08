-- Nothing in this migration refers to the new values, so ADD VALUE is safe
-- inside the single migration transaction.
ALTER TYPE workspace_agent_script_timing_status ADD VALUE IF NOT EXISTS 'skipped';
ALTER TYPE workspace_agent_script_timing_status ADD VALUE IF NOT EXISTS 'canceled';

COMMENT ON TYPE workspace_agent_script_timing_status IS 'The terminal outcome of a script execution.';
