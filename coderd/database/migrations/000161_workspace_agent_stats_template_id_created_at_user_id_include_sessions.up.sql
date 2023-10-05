DROP INDEX workspace_agent_stats_template_id_created_at_user_id_idx;

CREATE INDEX workspace_agent_stats_template_id_created_at_user_id_idx ON workspace_agent_stats (template_id, created_at, user_id) INCLUDE (session_count_vscode, session_count_jetbrains, session_count_reconnecting_pty, session_count_ssh, connection_median_latency_ms) WHERE connection_count > 0;

COMMENT ON INDEX workspace_agent_stats_template_id_created_at_user_id_idx IS 'Support index for template insights endpoint to build interval reports faster.';
