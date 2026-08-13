ALTER TABLE workspace_agent_stats
	ADD COLUMN session_count_vscode bigint DEFAULT 0 NOT NULL,
	ADD COLUMN session_count_jetbrains bigint DEFAULT 0 NOT NULL,
	ADD COLUMN session_count_reconnecting_pty bigint DEFAULT 0 NOT NULL,
	ADD COLUMN session_count_ssh bigint DEFAULT 0 NOT NULL;

-- Restore the four known session counts. Other keys are discarded.
UPDATE workspace_agent_stats
SET
	session_count_vscode = COALESCE((session_counts ->> 'vscode')::bigint, 0),
	session_count_jetbrains = COALESCE((session_counts ->> 'jetbrains')::bigint, 0),
	session_count_reconnecting_pty = COALESCE((session_counts ->> 'reconnecting_pty')::bigint, 0),
	session_count_ssh = COALESCE((session_counts ->> 'ssh')::bigint, 0);

DROP INDEX workspace_agent_stats_template_id_created_at_user_id_idx;

ALTER TABLE workspace_agent_stats
	DROP COLUMN session_counts;

CREATE INDEX workspace_agent_stats_template_id_created_at_user_id_idx ON workspace_agent_stats USING btree (template_id, created_at, user_id) INCLUDE (session_count_vscode, session_count_jetbrains, session_count_reconnecting_pty, session_count_ssh, connection_median_latency_ms) WHERE (connection_count > 0);

COMMENT ON INDEX workspace_agent_stats_template_id_created_at_user_id_idx IS 'Support index for template insights endpoint to build interval reports faster.';
