ALTER TABLE workspace_agent_logs RENAME TO workspace_agent_startup_logs;
ALTER TABLE workspace_agent_startup_logs DROP COLUMN source;
DROP TYPE workspace_agent_log_source;
ALTER TABLE workspace_agents RENAME COLUMN logs_overflowed TO startup_logs_overflowed;
ALTER TABLE workspace_agents RENAME COLUMN logs_length TO startup_logs_length;
ALTER TABLE workspace_agents RENAME CONSTRAINT max_logs_length TO max_startup_logs_length;
