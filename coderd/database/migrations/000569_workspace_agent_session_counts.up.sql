-- No DEFAULT on count: an insert omitting it is a bug and must fail.
CREATE TABLE workspace_agent_session_counts (
	workspace_agent_stats_id uuid NOT NULL REFERENCES workspace_agent_stats (id) ON DELETE CASCADE,
	created_at timestamptz NOT NULL,
	app_name text NOT NULL,
	count bigint NOT NULL,
	PRIMARY KEY (workspace_agent_stats_id, app_name)
);

COMMENT ON TABLE workspace_agent_session_counts IS 'Per-app session counts for each workspace agent stats row; rows are removed with their parent.';
COMMENT ON COLUMN workspace_agent_session_counts.created_at IS 'Copied from the parent stats row so time-windowed queries can prune this table without joining.';
COMMENT ON COLUMN workspace_agent_session_counts.app_name IS 'App name as reported by the client, canonicalized at ingestion (lowercased, hyphens folded to underscores). Stored ungrouped; families are applied at read time.';

-- Backfill so rollups and deployment stats see no gap during upgrade. One day
-- covers every window that reads raw stats, and caps the migration when the
-- purge or the rollup has fallen behind and the table holds months.
INSERT INTO workspace_agent_session_counts (workspace_agent_stats_id, created_at, app_name, count)
SELECT s.id, s.created_at, v.app_name, v.count
FROM workspace_agent_stats s
CROSS JOIN LATERAL (
	VALUES
		('vscode', s.session_count_vscode),
		('jetbrains', s.session_count_jetbrains),
		('reconnecting_pty', s.session_count_reconnecting_pty),
		('ssh', s.session_count_ssh)
) v(app_name, count)
WHERE v.count > 0
	AND s.created_at >= NOW() - '1 day'::interval;

-- Rows arrive in time order and purge oldest-first, so a BRIN index stays
-- tiny and lets windowed reads prune.
CREATE INDEX workspace_agent_session_counts_created_at_idx ON workspace_agent_session_counts USING brin (created_at);

-- Recreate the index without the dropped columns in its INCLUDE list.
DROP INDEX workspace_agent_stats_template_id_created_at_user_id_idx;

ALTER TABLE workspace_agent_stats
	DROP COLUMN session_count_vscode,
	DROP COLUMN session_count_jetbrains,
	DROP COLUMN session_count_reconnecting_pty,
	DROP COLUMN session_count_ssh;

CREATE INDEX workspace_agent_stats_template_id_created_at_user_id_idx ON workspace_agent_stats USING btree (template_id, created_at, user_id) INCLUDE (connection_median_latency_ms) WHERE (connection_count > 0);

COMMENT ON INDEX workspace_agent_stats_template_id_created_at_user_id_idx IS 'Support index for template insights endpoint to build interval reports faster.';
