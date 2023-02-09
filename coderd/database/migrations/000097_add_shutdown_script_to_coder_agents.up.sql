ALTER TABLE workspace_agents ADD COLUMN shutdown_script varchar(65534);

COMMENT ON COLUMN workspace_agents.shutdown_script IS 'Script that is executed before the agent is stopped.';

-- Disable shutdown script timeouts by default.
ALTER TABLE workspace_agents ADD COLUMN shutdown_script_timeout_seconds int4 NOT NULL DEFAULT 0;

COMMENT ON COLUMN workspace_agents.shutdown_script_timeout_seconds IS 'The number of seconds to wait for the shutdown script to complete. If the script does not complete within this time, the agent lifecycle will be marked as shutdown_timeout.';

-- Add enum fields
ALTER TYPE workspace_agent_lifecycle_state ADD VALUE 'shutting_down';
ALTER TYPE workspace_agent_lifecycle_state ADD VALUE 'shutdown_timeout';
ALTER TYPE workspace_agent_lifecycle_state ADD VALUE 'shutdown_error';
ALTER TYPE workspace_agent_lifecycle_state ADD VALUE 'off';
