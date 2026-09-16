LOCK TABLE workspace_agent_stats IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
	latest_rollup_start timestamptz;
	backlog_start timestamptz;
	backlog_end timestamptz;
BEGIN
	SELECT MAX(start_time) INTO latest_rollup_start FROM template_usage_stats;

	-- Every row the rollup has not consumed yet has to be converted below, and
	-- the table stays locked for all of them. Measure the rollup against the
	-- data rather than against the clock: a deployment shut down over a weekend
	-- records nothing while it is off, and an idle deployment leaves the
	-- watermark behind because there is nothing to roll up. Neither has a
	-- backlog, and neither takes noticeably longer to convert.
	SELECT MIN(created_at), MAX(created_at)
	INTO backlog_start, backlog_end
	FROM workspace_agent_stats
	WHERE (latest_rollup_start IS NULL OR created_at >= latest_rollup_start)
		AND (
			session_count_vscode > 0
			OR session_count_jetbrains > 0
			OR session_count_reconnecting_pty > 0
			OR session_count_ssh > 0
		);

	IF backlog_end - backlog_start > interval '24 hours' THEN
		RAISE WARNING 'migration 000590 found % of workspace agent stats that template usage stats never rolled up', justify_hours(backlog_end - backlog_start)
			USING DETAIL = format(
				'This migration converts every workspace_agent_stats row the rollup has not consumed, %s through %s, and holds ACCESS EXCLUSIVE on the table until it finishes. Expect the migration to take longer than usual; do not interrupt it.',
				backlog_start, backlog_end
			),
			HINT = 'A backlog this wide usually means the rollup is failing or blocked: after the upgrade, check coderd logs for "failed to rollup data", and check pg_locks for a session holding the rollup advisory lock.';
	END IF;
END
$$;

ALTER TABLE workspace_agent_stats
	ADD COLUMN session_counts jsonb DEFAULT '{}'::jsonb NOT NULL;

COMMENT ON COLUMN workspace_agent_stats.session_counts IS 'Positive session counts keyed by the canonical app name reported by the agent.';

-- Convert the window DeleteOldWorkspaceAgentStats retains plus everything the
-- rollup has not consumed yet, which is what the rollups and operational
-- statistics still read. Anything older has already been rolled up. When
-- nothing has ever rolled up there is no watermark to fall back on, so convert
-- every row rather than roll old activity up as zero minutes later.
UPDATE workspace_agent_stats
SET session_counts = jsonb_strip_nulls(jsonb_build_object(
	'vscode', CASE WHEN session_count_vscode > 0 THEN session_count_vscode END,
	'jetbrains', CASE WHEN session_count_jetbrains > 0 THEN session_count_jetbrains END,
	'reconnecting_pty', CASE WHEN session_count_reconnecting_pty > 0 THEN session_count_reconnecting_pty END,
	'ssh', CASE WHEN session_count_ssh > 0 THEN session_count_ssh END
))
WHERE created_at >= (
	SELECT COALESCE(MAX(start_time) - interval '1 day', '-infinity'::timestamptz)
	FROM template_usage_stats
)
	AND (
		session_count_vscode > 0
		OR session_count_jetbrains > 0
		OR session_count_reconnecting_pty > 0
		OR session_count_ssh > 0
	);

-- Recreate the index without the dropped columns in its INCLUDE list.
DROP INDEX workspace_agent_stats_template_id_created_at_user_id_idx;

ALTER TABLE workspace_agent_stats
	DROP COLUMN session_count_vscode,
	DROP COLUMN session_count_jetbrains,
	DROP COLUMN session_count_reconnecting_pty,
	DROP COLUMN session_count_ssh;

CREATE INDEX workspace_agent_stats_template_id_created_at_user_id_idx ON workspace_agent_stats USING btree (template_id, created_at, user_id) INCLUDE (connection_median_latency_ms) WHERE (connection_count > 0);

COMMENT ON INDEX workspace_agent_stats_template_id_created_at_user_id_idx IS 'Support index for template insights endpoint to build interval reports faster.';
