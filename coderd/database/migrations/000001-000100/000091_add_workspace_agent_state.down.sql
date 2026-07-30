ALTER TABLE workspace_agents DROP COLUMN startup_script_timeout_seconds;
ALTER TABLE workspace_agents DROP COLUMN delay_login_until_ready;

ALTER TABLE workspace_agents DROP COLUMN lifecycle_state;

DROP TYPE workspace_agent_lifecycle_state;
