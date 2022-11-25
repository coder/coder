ALTER TABLE workspace_agents ADD COLUMN shutdown_script varchar(65534);

COMMENT ON COLUMN workspace_agents.shutdown_script IS 'Script that is executed before the agent is stopped.';
