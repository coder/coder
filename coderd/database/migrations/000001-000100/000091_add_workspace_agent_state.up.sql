CREATE TYPE workspace_agent_lifecycle_state AS ENUM ('created', 'starting', 'start_timeout', 'start_error', 'ready');

-- Set all existing workspace agents to 'ready' so that only newly created agents will be in the 'created' state.
ALTER TABLE workspace_agents ADD COLUMN lifecycle_state workspace_agent_lifecycle_state NOT NULL DEFAULT 'ready';
-- Change the default for newly created agents.
ALTER TABLE workspace_agents ALTER COLUMN lifecycle_state SET DEFAULT 'created';

COMMENT ON COLUMN workspace_agents.lifecycle_state IS 'The current lifecycle state reported by the workspace agent.';

-- Set default values that conform to current behavior.
-- Allow logins immediately after agent connect.
ALTER TABLE workspace_agents ADD COLUMN delay_login_until_ready boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN workspace_agents.delay_login_until_ready IS 'If true, the agent will delay logins until it is ready (e.g. executing startup script has ended).';

-- Disable startup script timeouts by default.
ALTER TABLE workspace_agents ADD COLUMN startup_script_timeout_seconds int4 NOT NULL DEFAULT 0;

COMMENT ON COLUMN workspace_agents.startup_script_timeout_seconds IS 'The number of seconds to wait for the startup script to complete. If the script does not complete within this time, the agent lifecycle will be marked as start_timeout.';
