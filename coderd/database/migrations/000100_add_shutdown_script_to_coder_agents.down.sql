ALTER TABLE workspace_agents DROP COLUMN shutdown_script;

ALTER TABLE workspace_agents DROP COLUMN shutdown_script_timeout_seconds;

-- We can't drop values from enums, so we have to create a new one and convert the data.
UPDATE workspace_agents SET lifecycle_state = 'ready' WHERE lifecycle_state IN ('shutting_down', 'shutdown_timeout', 'shutdown_error', 'off');
ALTER TYPE workspace_agent_lifecycle_state RENAME TO workspace_agent_lifecycle_state_old;
CREATE TYPE workspace_agent_lifecycle_state AS ENUM ('created', 'starting', 'start_timeout', 'start_error', 'ready');
ALTER TABLE workspace_agents ALTER COLUMN lifecycle_state DROP DEFAULT;
ALTER TABLE workspace_agents ALTER COLUMN lifecycle_state TYPE workspace_agent_lifecycle_state USING lifecycle_state::text::workspace_agent_lifecycle_state;
ALTER TABLE workspace_agents ALTER COLUMN lifecycle_state SET DEFAULT 'created';
DROP TYPE workspace_agent_lifecycle_state_old;
