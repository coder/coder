CREATE TYPE workspace_agent_state AS ENUM ('starting', 'start_timeout', 'start_error', 'ready');

ALTER TABLE workspace_agents ADD COLUMN state workspace_agent_state NULL DEFAULT NULL;

COMMENT ON COLUMN workspace_agents.state IS 'The current state of the workspace agent.';
