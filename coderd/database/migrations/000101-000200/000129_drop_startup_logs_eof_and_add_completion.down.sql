ALTER TABLE workspace_agents
	DROP COLUMN started_at,
	DROP COLUMN ready_at;

-- We won't bring back log entries where eof = TRUE, but this doesn't matter
-- as the implementation doesn't require it and hasn't been part of a release.
ALTER TABLE workspace_agent_startup_logs ADD COLUMN eof boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN workspace_agent_startup_logs.eof IS 'End of file reached';
