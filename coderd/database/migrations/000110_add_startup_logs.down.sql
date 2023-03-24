DROP TABLE workspace_agent_startup_logs;
ALTER TABLE ONLY workspace_agents
	DROP COLUMN startup_logs_length,
	DROP COLUMN startup_logs_overflowed;
