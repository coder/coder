LOCK TABLE workspace_agent_stats IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
	latest_rollup_start timestamptz;
	migration_cutoff timestamptz;
BEGIN
	SELECT MAX(start_time) INTO latest_rollup_start FROM template_usage_stats;
	migration_cutoff := COALESCE(latest_rollup_start - interval '1 day', statement_timestamp() - interval '180 days');

	IF (latest_rollup_start IS NULL OR latest_rollup_start < statement_timestamp() - interval '24 hours')
		AND EXISTS (
			SELECT 1
			FROM workspace_agent_stats
			WHERE created_at >= migration_cutoff
				AND (
					session_count_vscode > 0
					OR session_count_jetbrains > 0
					OR session_count_reconnecting_pty > 0
					OR session_count_ssh > 0
				)
		)
	THEN
		RAISE EXCEPTION 'migration 000569 requires template usage stats rolled up within the last 24 hours; run the previous Coder version until template usage stats roll up, then retry the upgrade'
			USING DETAIL = format(
				'Latest template_usage_stats.start_time: %s.',
				COALESCE(latest_rollup_start::text, 'none')
			),
			HINT = 'Check coderd logs for "failed to rollup data" if the timestamp does not advance.';
	END IF;
END
$$;

ALTER TABLE workspace_agent_stats
	ADD COLUMN session_counts jsonb DEFAULT '{}'::jsonb NOT NULL;

COMMENT ON COLUMN workspace_agent_stats.session_counts IS 'Positive session counts keyed by the canonical app name reported by the agent.';

-- Convert the raw window still used by rollups and operational statistics.
-- Older usage is already in template usage rollups. The guard requires a recent
-- rollup when retained rows need conversion.
UPDATE workspace_agent_stats
SET session_counts = jsonb_strip_nulls(jsonb_build_object(
	'vscode', CASE WHEN session_count_vscode > 0 THEN session_count_vscode END,
	'jetbrains', CASE WHEN session_count_jetbrains > 0 THEN session_count_jetbrains END,
	'reconnecting_pty', CASE WHEN session_count_reconnecting_pty > 0 THEN session_count_reconnecting_pty END,
	'ssh', CASE WHEN session_count_ssh > 0 THEN session_count_ssh END
))
WHERE created_at >= (
	SELECT COALESCE(MAX(start_time) - interval '1 day', statement_timestamp() - interval '180 days')
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
