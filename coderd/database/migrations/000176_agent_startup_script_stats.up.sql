ALTER TABLE workspace_agent_stats ADD COLUMN startup_script_ns BIGINT NOT NULL DEFAULT 0;
ALTER TABLE workspace_agent_stats ADD COLUMN startup_script_success BOOL NOT NULL DEFAULT false;

COMMENT ON COLUMN workspace_agent_stats.startup_script_ns IS 'The time it took to run the startup script in nanoseconds. If set to 0, the startup script was not run.';
COMMENT ON COLUMN workspace_agent_stats.startup_script_success IS 'Whether the startup script was run successfully. Will be false if the duration is 0, but the script has not been run.';
