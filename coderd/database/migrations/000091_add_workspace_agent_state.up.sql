CREATE TYPE workspace_agent_lifecycle_state AS ENUM ('created', 'starting', 'start_timeout', 'start_error', 'ready');

ALTER TABLE workspace_agents ADD COLUMN lifecycle_state workspace_agent_lifecycle_state NOT NULL DEFAULT 'created';

COMMENT ON COLUMN workspace_agents.lifecycle_state IS 'The current lifecycle state of the workspace agent.';
