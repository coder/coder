ALTER TABLE	workspace_agent_stats ADD COLUMN connection_median_latency_ms bigint DEFAULT -1 NOT NULL;
ALTER TABLE	workspace_agent_stats ADD COLUMN session_count_vscode bigint DEFAULT 0 NOT NULL;
ALTER TABLE	workspace_agent_stats ADD COLUMN session_count_jetbrains bigint DEFAULT 0 NOT NULL;
ALTER TABLE	workspace_agent_stats ADD COLUMN session_count_reconnecting_pty bigint DEFAULT 0 NOT NULL;
ALTER TABLE	workspace_agent_stats ADD COLUMN session_count_ssh bigint DEFAULT 0 NOT NULL;
