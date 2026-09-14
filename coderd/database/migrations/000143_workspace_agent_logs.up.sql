CREATE TYPE workspace_agent_log_source AS ENUM ('startup_script', 'shutdown_script', 'kubernetes_logs', 'envbox', 'envbuilder', 'external');
ALTER TABLE workspace_agent_startup_logs RENAME TO workspace_agent_logs;
ALTER TABLE workspace_agent_logs ADD COLUMN source workspace_agent_log_source NOT NULL DEFAULT 'startup_script';
ALTER TABLE workspace_agents RENAME COLUMN startup_logs_overflowed TO logs_overflowed;
ALTER TABLE workspace_agents RENAME COLUMN startup_logs_length TO logs_length;
ALTER TABLE workspace_agents RENAME CONSTRAINT max_startup_logs_length TO max_logs_length;
