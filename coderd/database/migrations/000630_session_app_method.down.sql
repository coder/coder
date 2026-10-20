DROP FUNCTION session_count_apps(jsonb);

ALTER TABLE workspace_agent_stats
	DROP CONSTRAINT workspace_agent_stats_session_counts_check;

-- Rows the up migration left flat are skipped.
UPDATE workspace_agent_stats
SET session_counts = (
	SELECT jsonb_object_agg(app_name, sessions)
	FROM (
		SELECT apps.key AS app_name, SUM(apps.value::bigint) AS sessions
		FROM jsonb_each(session_counts) AS methods, jsonb_each_text(methods.value) AS apps
		GROUP BY apps.key
	) AS by_app
)
WHERE jsonb_path_exists(session_counts, '$.* ? (@.type() == "object")');

COMMENT ON COLUMN workspace_agent_stats.session_counts IS 'Positive session counts keyed by the canonical app name reported by the agent.';

-- Overlap between methods is not stored, so cap merged minutes at the bucket.
CREATE TEMPORARY TABLE session_apps_by_name ON COMMIT DROP AS
SELECT start_time, template_id, user_id, app_name, LEAST(SUM(usage_mins), 30)::smallint AS usage_mins
FROM template_usage_stats_session_apps
GROUP BY start_time, template_id, user_id, app_name;

DELETE FROM template_usage_stats_session_apps;

ALTER TABLE template_usage_stats_session_apps
	DROP COLUMN method,
	ADD PRIMARY KEY (start_time, user_id, template_id, app_name);

INSERT INTO template_usage_stats_session_apps (start_time, template_id, user_id, app_name, usage_mins)
SELECT start_time, template_id, user_id, app_name, usage_mins
FROM session_apps_by_name;
