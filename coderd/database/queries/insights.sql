-- name: GetUserLatencyInsights :many
-- GetUserLatencyInsights returns the median and 95th percentile connection
-- latency that users have experienced. The result can be filtered on
-- template_ids, meaning only user data from workspaces based on those templates
-- will be included.
SELECT
	workspace_agent_stats.user_id,
	users.username,
	users.avatar_url,
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
GROUP BY workspace_agent_stats.user_id, users.username, users.avatar_url
ORDER BY user_id ASC;

-- name: GetTemplateInsights :one
-- GetTemplateInsights has a granularity of 5 minutes where if a session/app was
-- in use during a minute, we will add 5 minutes to the total usage for that
-- session/app (per user).
WITH agent_stats_by_interval_and_user AS (
	SELECT
		date_trunc('minute', was.created_at),
		was.user_id,
		array_agg(was.template_id) AS template_ids,
		CASE WHEN SUM(was.session_count_vscode) > 0 THEN 60 ELSE 0 END AS usage_vscode_seconds,
		CASE WHEN SUM(was.session_count_jetbrains) > 0 THEN 60 ELSE 0 END AS usage_jetbrains_seconds,
		CASE WHEN SUM(was.session_count_reconnecting_pty) > 0 THEN 60 ELSE 0 END AS usage_reconnecting_pty_seconds,
		CASE WHEN SUM(was.session_count_ssh) > 0 THEN 60 ELSE 0 END AS usage_ssh_seconds
	FROM workspace_agent_stats was
	WHERE
		was.created_at >= @start_time::timestamptz
		AND was.created_at < @end_time::timestamptz
		AND was.connection_count > 0
		AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN was.template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
	GROUP BY date_trunc('minute', was.created_at), was.user_id
), template_ids AS (
	SELECT array_agg(DISTINCT template_id) AS ids
	FROM agent_stats_by_interval_and_user, unnest(template_ids) template_id
	WHERE template_id IS NOT NULL
)

SELECT
	COALESCE((SELECT ids FROM template_ids), '{}')::uuid[] AS template_ids,
	-- Return IDs so we can combine this with GetTemplateAppInsights.
	COALESCE(array_agg(DISTINCT user_id), '{}')::uuid[] AS active_user_ids,
	COALESCE(SUM(usage_vscode_seconds), 0)::bigint AS usage_vscode_seconds,
	COALESCE(SUM(usage_jetbrains_seconds), 0)::bigint AS usage_jetbrains_seconds,
	COALESCE(SUM(usage_reconnecting_pty_seconds), 0)::bigint AS usage_reconnecting_pty_seconds,
	COALESCE(SUM(usage_ssh_seconds), 0)::bigint AS usage_ssh_seconds
FROM agent_stats_by_interval_and_user;

-- name: GetTemplateAppInsights :many
-- GetTemplateAppInsights returns the aggregate usage of each app in a given
-- timeframe. The result can be filtered on template_ids, meaning only user data
-- from workspaces based on those templates will be included.
WITH app_stats_by_user_and_agent AS (
	SELECT
		s.start_time,
		60 as seconds,
		w.template_id,
		was.user_id,
		was.agent_id,
		was.access_method,
		was.slug_or_port,
		wa.display_name,
		wa.icon,
		(wa.slug IS NOT NULL)::boolean AS is_app
	FROM workspace_app_stats was
	JOIN workspaces w ON (
		w.id = was.workspace_id
		AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN w.template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
	)
	-- We do a left join here because we want to include user IDs that have used
	-- e.g. ports when counting active users.
	LEFT JOIN workspace_apps wa ON (
		wa.agent_id = was.agent_id
		AND wa.slug = was.slug_or_port
	)
	-- This table contains both 1 minute entries and >1 minute entries,
	-- to calculate this with our uniqueness constraints, we generate series
	-- for the longer intervals.
	CROSS JOIN LATERAL generate_series(
		date_trunc('minute', was.session_started_at),
		-- Subtract 1 microsecond to avoid creating an extra series.
		date_trunc('minute', was.session_ended_at - '1 microsecond'::interval),
		'1 minute'::interval
	) s(start_time)
	WHERE
		s.start_time >= @start_time::timestamptz
		-- Subtract one minute because the series only contains the start time.
		AND s.start_time < (@end_time::timestamptz) - '1 minute'::interval
	GROUP BY s.start_time, w.template_id, was.user_id, was.agent_id, was.access_method, was.slug_or_port, wa.display_name, wa.icon, wa.slug
)

SELECT
	array_agg(DISTINCT template_id)::uuid[] AS template_ids,
	-- Return IDs so we can combine this with GetTemplateInsights.
	array_agg(DISTINCT user_id)::uuid[] AS active_user_ids,
	access_method,
	slug_or_port,
	display_name,
	icon,
	is_app,
	SUM(seconds) AS usage_seconds
FROM app_stats_by_user_and_agent
GROUP BY access_method, slug_or_port, display_name, icon, is_app;

-- name: GetTemplateInsightsByInterval :many
-- GetTemplateInsightsByInterval returns all intervals between start and end
-- time, if end time is a partial interval, it will be included in the results and
-- that interval will be shorter than a full one. If there is no data for a selected
-- interval/template, it will be included in the results with 0 active users.
WITH ts AS (
	SELECT
		d::timestamptz AS from_,
		CASE
			WHEN (d::timestamptz + (@interval_days::int || ' day')::interval) <= @end_time::timestamptz
			THEN (d::timestamptz + (@interval_days::int || ' day')::interval)
			ELSE @end_time::timestamptz
		END AS to_
	FROM
		-- Subtract 1 microsecond from end_time to avoid including the next interval in the results.
		generate_series(@start_time::timestamptz, (@end_time::timestamptz) - '1 microsecond'::interval, (@interval_days::int || ' day')::interval) AS d
), unflattened_usage_by_interval AS (
	-- We select data from both workspace agent stats and workspace app stats to
	-- get a complete picture of usage. This matches how usage is calculated by
	-- the combination of GetTemplateInsights and GetTemplateAppInsights. We use
	-- a union all to avoid a costly distinct operation.
	--
	-- Note that one query must perform a left join so that all intervals are
	-- present at least once.
	SELECT
		ts.*,
		was.template_id,
		was.user_id
	FROM ts
	LEFT JOIN workspace_agent_stats was ON (
		was.created_at >= ts.from_
		AND was.created_at < ts.to_
		AND was.connection_count > 0
		AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN was.template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
	)
	GROUP BY ts.from_, ts.to_, was.template_id, was.user_id

	UNION ALL

	SELECT
		ts.*,
		w.template_id,
		was.user_id
	FROM ts
	JOIN workspace_app_stats was ON (
		(was.session_started_at >= ts.from_ AND was.session_started_at < ts.to_)
		OR (was.session_ended_at > ts.from_ AND was.session_ended_at < ts.to_)
		OR (was.session_started_at < ts.from_ AND was.session_ended_at >= ts.to_)
	)
	JOIN workspaces w ON (
		w.id = was.workspace_id
		AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN w.template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
	)
	GROUP BY ts.from_, ts.to_, w.template_id, was.user_id
)

SELECT
	from_ AS start_time,
	to_ AS end_time,
	array_remove(array_agg(DISTINCT template_id), NULL)::uuid[] AS template_ids,
	COUNT(DISTINCT user_id) AS active_users
FROM unflattened_usage_by_interval
GROUP BY from_, to_;

-- name: GetTemplateParameterInsights :many
-- GetTemplateParameterInsights does for each template in a given timeframe,
-- look for the latest workspace build (for every workspace) that has been
-- created in the timeframe and return the aggregate usage counts of parameter
-- values.
WITH latest_workspace_builds AS (
	SELECT
		wb.id,
		wbmax.template_id,
		wb.template_version_id
	FROM (
		SELECT
			tv.template_id, wbmax.workspace_id, MAX(wbmax.build_number) as max_build_number
		FROM workspace_builds wbmax
		JOIN template_versions tv ON (tv.id = wbmax.template_version_id)
		WHERE
			wbmax.created_at >= @start_time::timestamptz
			AND wbmax.created_at < @end_time::timestamptz
			AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN tv.template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
		GROUP BY tv.template_id, wbmax.workspace_id
	) wbmax
	JOIN workspace_builds wb ON (
		wb.workspace_id = wbmax.workspace_id
		AND wb.build_number = wbmax.max_build_number
	)
), unique_template_params AS (
	SELECT
		ROW_NUMBER() OVER () AS num,
		array_agg(DISTINCT wb.template_id)::uuid[] AS template_ids,
		array_agg(wb.id)::uuid[] AS workspace_build_ids,
		tvp.name,
		tvp.type,
		tvp.display_name,
		tvp.description,
		tvp.options
	FROM latest_workspace_builds wb
	JOIN template_version_parameters tvp ON (tvp.template_version_id = wb.template_version_id)
	GROUP BY tvp.name, tvp.type, tvp.display_name, tvp.description, tvp.options
)

SELECT
	utp.num,
	utp.template_ids,
	utp.name,
	utp.type,
	utp.display_name,
	utp.description,
	utp.options,
	wbp.value,
	COUNT(wbp.value) AS count
FROM unique_template_params utp
JOIN workspace_build_parameters wbp ON (utp.workspace_build_ids @> ARRAY[wbp.workspace_build_id] AND utp.name = wbp.name)
GROUP BY utp.num, utp.template_ids, utp.name, utp.type, utp.display_name, utp.description, utp.options, wbp.value;
