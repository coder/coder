-- Block writes but not reads: a coderd still on the previous release would
-- otherwise insert flat session counts that the conversion below misses.
LOCK TABLE workspace_agent_stats IN EXCLUSIVE MODE;

DO $$
DECLARE
	backlog interval;
BEGIN
	SELECT MAX(created_at) - MIN(created_at) INTO backlog
	FROM workspace_agent_stats
	WHERE created_at >= (SELECT COALESCE(MAX(start_time), '-infinity') FROM template_usage_stats)
		AND session_counts <> '{}'::jsonb;

	IF backlog > interval '24 hours' THEN
		RAISE WARNING 'migration 000634 found % of workspace agent stats that template usage stats never rolled up', justify_hours(backlog)
			USING DETAIL = 'This migration converts every row the rollup has not consumed and blocks writes to workspace_agent_stats until it finishes. Expect it to take longer than usual; do not interrupt it.',
			HINT = 'A backlog this wide usually means the rollup is failing: after the upgrade, check coderd logs for "failed to rollup data".';
	END IF;
END
$$;

-- Convert the window DeleteOldWorkspaceAgentStats retains, which is all that
-- the rollup and operational statistics still read, or every row if nothing
-- has rolled up yet. Older rows stay flat until they are purged.
UPDATE workspace_agent_stats
SET session_counts = (
	SELECT jsonb_object_agg(method, apps)
	FROM (
		SELECT
			CASE WHEN key IN ('reconnecting_pty', 'port_forwarding') THEN key ELSE 'ssh' END AS method,
			jsonb_object_agg(key, value) AS apps
		FROM jsonb_each(session_counts)
		GROUP BY 1
	) AS by_method
)
WHERE created_at >= (
	SELECT COALESCE(MAX(start_time) - interval '1 day', '-infinity')
	FROM template_usage_stats
)
	AND session_counts <> '{}'::jsonb;

-- Rejects the previous release's flat writes, and keys that are not session
-- methods, either of which would stall the rollup. NOT VALID skips the
-- unconverted rows outside the window, which nothing reads. It locks
-- workspace_agent_stats before template_usage_stats_session_apps, the order
-- the rollup uses, so it cannot deadlock with a previous release's rollup.
-- From here, reads of workspace_agent_stats wait for the commit.
ALTER TABLE workspace_agent_stats
	ADD CONSTRAINT workspace_agent_stats_session_counts_check
		CHECK (NOT jsonb_path_exists(session_counts, '$.keyvalue() ? (@.value.type() != "object" || !(@.key == "ssh" || @.key == "reconnecting_pty" || @.key == "port_forwarding"))')) NOT VALID;

COMMENT ON COLUMN workspace_agent_stats.session_counts IS 'Positive session counts keyed by connection method, then by the canonical app name reported by the agent.';

-- The default fills existing rows without rewriting them; rows recorded
-- without a method derive it from the app name.
ALTER TABLE template_usage_stats_session_apps
	ADD COLUMN method connection_log_method NOT NULL DEFAULT 'ssh';

UPDATE template_usage_stats_session_apps
SET method = app_name::connection_log_method
WHERE app_name IN ('reconnecting_pty', 'port_forwarding');

ALTER TABLE template_usage_stats_session_apps
	ALTER COLUMN method DROP DEFAULT,
	ADD CONSTRAINT template_usage_stats_session_apps_method_check
		CHECK (method IN ('ssh', 'reconnecting_pty', 'port_forwarding')),
	DROP CONSTRAINT template_usage_stats_session_apps_pkey,
	ADD PRIMARY KEY (start_time, user_id, template_id, method, app_name);

COMMENT ON COLUMN template_usage_stats_session_apps.method IS 'How the app connected, derived from the app name.';

-- Expands session_counts for UpsertTemplateUsageStats. STRICT keeps the
-- planner from inlining the body, whose nested set-returning calls estimate
-- 10,000 rows per stat row instead of ROWS 4. That overestimate stops the
-- rollup hashing its NOT IN list and turns on JIT for large tables. STABLE
-- because the enum cast is stable.
CREATE FUNCTION session_count_apps(session_counts jsonb)
RETURNS TABLE (method connection_log_method, app_name text)
LANGUAGE sql STABLE STRICT PARALLEL SAFE ROWS 4 AS $$
	SELECT methods.key::connection_log_method, app_names.app_name
	FROM jsonb_each(session_counts) AS methods
	CROSS JOIN jsonb_object_keys(methods.value) AS app_names(app_name)
$$;

COMMENT ON FUNCTION session_count_apps(jsonb) IS 'Expands workspace_agent_stats.session_counts into one row per connection method and app name.';
