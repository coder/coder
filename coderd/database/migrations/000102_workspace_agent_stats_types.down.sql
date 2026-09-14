ALTER TABLE	workspace_agent_stats DROP COLUMN session_count_vscode,
	DROP COLUMN session_count_jetbrains,
	DROP COLUMN session_count_reconnecting_pty,
	DROP COLUMN session_count_ssh,
	DROP COLUMN connection_median_latency_ms;
