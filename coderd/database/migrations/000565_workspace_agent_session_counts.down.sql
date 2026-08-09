ALTER TABLE workspace_agent_stats
	ADD COLUMN session_count_vscode bigint DEFAULT 0 NOT NULL,
	ADD COLUMN session_count_jetbrains bigint DEFAULT 0 NOT NULL,
	ADD COLUMN session_count_reconnecting_pty bigint DEFAULT 0 NOT NULL,
	ADD COLUMN session_count_ssh bigint DEFAULT 0 NOT NULL;

-- Restore the four well-known session counts. Any other app name cannot be
-- represented in the old schema and is silently discarded.
UPDATE workspace_agent_stats was
SET
	session_count_vscode = COALESCE(sc.vscode, 0),
	session_count_jetbrains = COALESCE(sc.jetbrains, 0),
	session_count_reconnecting_pty = COALESCE(sc.reconnecting_pty, 0),
	session_count_ssh = COALESCE(sc.ssh, 0)
FROM (
	SELECT
		workspace_agent_stats_id,
		SUM(count) FILTER (WHERE app_name = 'vscode') AS vscode,
		SUM(count) FILTER (WHERE app_name = 'jetbrains') AS jetbrains,
		SUM(count) FILTER (WHERE app_name = 'reconnecting_pty') AS reconnecting_pty,
		SUM(count) FILTER (WHERE app_name = 'ssh') AS ssh
	FROM workspace_agent_session_counts
	GROUP BY workspace_agent_stats_id
) sc
WHERE sc.workspace_agent_stats_id = was.id;

DROP TABLE workspace_agent_session_counts;

DROP INDEX workspace_agent_stats_template_id_created_at_user_id_idx;

CREATE INDEX workspace_agent_stats_template_id_created_at_user_id_idx ON workspace_agent_stats USING btree (template_id, created_at, user_id) INCLUDE (session_count_vscode, session_count_jetbrains, session_count_reconnecting_pty, session_count_ssh, connection_median_latency_ms) WHERE (connection_count > 0);

COMMENT ON INDEX workspace_agent_stats_template_id_created_at_user_id_idx IS 'Support index for template insights endpoint to build interval reports faster.';
