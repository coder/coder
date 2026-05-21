ALTER TABLE workspace_agents
	ADD COLUMN connection_timeout_seconds integer NOT NULL DEFAULT 0;

COMMENT ON COLUMN workspace_agents.connection_timeout_seconds IS 'Connection timeout in seconds, 0 means disabled.';

ALTER TABLE workspace_agents
	ADD COLUMN troubleshooting_url text NOT NULL DEFAULT '';

COMMENT ON COLUMN workspace_agents.troubleshooting_url IS 'URL for troubleshooting the agent.';
