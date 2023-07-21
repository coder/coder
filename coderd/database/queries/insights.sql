-- name: GetUserLatencyInsights :many
-- GetUserLatencyInsights returns the median and 95th percentile connection
-- latency that users have experienced. The result can be filtered on
-- template_ids, meaning only user data from workspaces based on those templates
-- will be included.
SELECT
	workspace_agent_stats.user_id,
	users.username,
	array_agg(DISTINCT template_id)::uuid[] AS template_ids,
	coalesce((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY connection_median_latency_ms)), -1)::FLOAT AS workspace_connection_latency_50,
	coalesce((PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY connection_median_latency_ms)), -1)::FLOAT AS workspace_connection_latency_95
FROM workspace_agent_stats
JOIN users ON (users.id = workspace_agent_stats.user_id)
WHERE
	workspace_agent_stats.created_at >= @start_time
	AND workspace_agent_stats.created_at < @end_time
	AND workspace_agent_stats.connection_median_latency_ms > 0
	AND workspace_agent_stats.connection_count > 0
	AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
GROUP BY workspace_agent_stats.user_id, users.username
ORDER BY user_id ASC;

-- name: GetTemplateInsights :one
-- GetTemplateInsights has a granularity of 5 minutes where if a session/app was
-- in use, we will add 5 minutes to the total usage for that session (per user).
WITH d AS (
	-- Subtract 1 second from end_time to avoid including the next interval in the results.
	SELECT generate_series(@start_time::timestamptz, (@end_time::timestamptz) - '1 second'::interval, '5 minute'::interval) AS d
), ts AS (
	SELECT
		d::timestamptz AS from_,
		(d + '5 minute'::interval)::timestamptz AS to_,
		EXTRACT(epoch FROM '5 minute'::interval) AS seconds
	FROM d
), usage_by_user AS (
	SELECT
		ts.from_,
		ts.to_,
		was.user_id,
		array_agg(was.template_id) AS template_ids,
		CASE WHEN SUM(was.session_count_vscode) > 0 THEN ts.seconds ELSE 0 END AS usage_vscode_seconds,
		CASE WHEN SUM(was.session_count_jetbrains) > 0 THEN ts.seconds ELSE 0 END AS usage_jetbrains_seconds,
		CASE WHEN SUM(was.session_count_reconnecting_pty) > 0 THEN ts.seconds ELSE 0 END AS usage_reconnecting_pty_seconds,
		CASE WHEN SUM(was.session_count_ssh) > 0 THEN ts.seconds ELSE 0 END AS usage_ssh_seconds
	FROM ts
	JOIN workspace_agent_stats was ON (
		was.created_at >= ts.from_
		AND was.created_at < ts.to_
		AND was.connection_count > 0
		AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN was.template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
	)
	GROUP BY ts.from_, ts.to_, ts.seconds, was.user_id
), template_ids AS (
	SELECT array_agg(DISTINCT template_id) AS ids
	FROM usage_by_user, unnest(template_ids) template_id
	WHERE template_id IS NOT NULL
)

SELECT
	COALESCE((SELECT ids FROM template_ids), '{}')::uuid[] AS template_ids,
	COUNT(DISTINCT user_id) AS active_users,
	COALESCE(SUM(usage_vscode_seconds), 0)::bigint AS usage_vscode_seconds,
	COALESCE(SUM(usage_jetbrains_seconds), 0)::bigint AS usage_jetbrains_seconds,
	COALESCE(SUM(usage_reconnecting_pty_seconds), 0)::bigint AS usage_reconnecting_pty_seconds,
	COALESCE(SUM(usage_ssh_seconds), 0)::bigint AS usage_ssh_seconds
FROM usage_by_user;

-- name: GetTemplateDailyInsights :many
-- GetTemplateDailyInsights returns all daily intervals between start and end
-- time, if end time is a partial day, it will be included in the results and
-- that interval will be less than 24 hours. If there is no data for a selected
-- interval/template, it will be included in the results with 0 active users.
WITH d AS (
	-- sqlc workaround, use SELECT generate_series instead of SELECT * FROM generate_series.
	-- Subtract 1 second from end_time to avoid including the next interval in the results.
	SELECT generate_series(@start_time::timestamptz, (@end_time::timestamptz) - '1 second'::interval, '1 day'::interval) AS d
), ts AS (
	SELECT
		d::timestamptz AS from_,
		CASE WHEN (d + '1 day'::interval)::timestamptz <= @end_time::timestamptz THEN (d + '1 day'::interval)::timestamptz ELSE @end_time::timestamptz END AS to_
	FROM d
), usage_by_day AS (
	SELECT
		ts.*,
		was.user_id,
		array_agg(was.template_id) AS template_ids
	FROM ts
	LEFT JOIN workspace_agent_stats was ON (
		was.created_at >= ts.from_
		AND was.created_at < ts.to_
		AND was.connection_count > 0
		AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN was.template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
	)
	GROUP BY ts.from_, ts.to_, was.user_id
), template_ids AS (
	SELECT
		from_,
		array_agg(DISTINCT template_id) AS ids
	FROM usage_by_day, unnest(template_ids) template_id
	WHERE template_id IS NOT NULL
	GROUP BY from_, template_ids
)

SELECT
	from_ AS start_time,
	to_ AS end_time,
	COALESCE((SELECT template_ids.ids FROM template_ids WHERE template_ids.from_ = usage_by_day.from_), '{}')::uuid[] AS template_ids,
	COUNT(DISTINCT user_id) AS active_users
FROM usage_by_day
GROUP BY from_, to_;
