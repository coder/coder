BEGIN;

ALTER TABLE workspace_agents
	DROP COLUMN started_at,
	DROP COLUMN ready_at;

ALTER TABLE workspace_agent_startup_logs ADD COLUMN eof boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN workspace_agent_startup_logs.eof IS 'End of file reached';

COMMIT;
