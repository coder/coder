DELETE FROM workspace_agent_startup_logs WHERE eof IS TRUE;

ALTER TABLE workspace_agent_startup_logs DROP COLUMN eof;

ALTER TABLE workspace_agents
	ADD COLUMN started_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
	ADD COLUMN ready_at TIMESTAMP WITH TIME ZONE DEFAULT NULL;

COMMENT ON COLUMN workspace_agents.started_at IS 'The time the agent entered the starting lifecycle state';
COMMENT ON COLUMN workspace_agents.ready_at IS 'The time the agent entered the ready or start_error lifecycle state';
