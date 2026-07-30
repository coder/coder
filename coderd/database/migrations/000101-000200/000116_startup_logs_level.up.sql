ALTER TABLE workspace_agent_startup_logs
	ADD COLUMN level log_level NOT NULL DEFAULT 'info'::log_level;
