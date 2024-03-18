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

-- name: GetUserActivityInsights :many
-- GetUserActivityInsights returns the ranking with top active users.
-- The result can be filtered on template_ids, meaning only user data from workspaces
-- based on those templates will be included.
-- Note: When selecting data from multiple templates or the entire deployment,
-- be aware that it may lead to an increase in "usage" numbers (cumulative). In such cases,
-- users may be counted multiple times for the same time interval if they have used multiple templates
-- simultaneously.
WITH app_stats AS (
	SELECT
		s.start_time,
		was.user_id,
		w.template_id,
		60 as seconds
	FROM workspace_app_stats was
	JOIN workspaces w ON (
		w.id = was.workspace_id
		AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN w.template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
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
	GROUP BY s.start_time, w.template_id, was.user_id
), session_stats AS (
	SELECT
		date_trunc('minute', was.created_at) as start_time,
		was.user_id,
		was.template_id,
		CASE WHEN
			SUM(was.session_count_vscode) > 0 OR
			SUM(was.session_count_jetbrains) > 0 OR
			SUM(was.session_count_reconnecting_pty) > 0 OR
			SUM(was.session_count_ssh) > 0
		THEN 60 ELSE 0 END as seconds
	FROM workspace_agent_stats was
	WHERE
		was.created_at >= @start_time::timestamptz
		AND was.created_at < @end_time::timestamptz
		AND was.connection_count > 0
		AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN was.template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
	GROUP BY date_trunc('minute', was.created_at), was.user_id, was.template_id
), combined_stats AS (
	SELECT
		user_id,
		template_id,
		start_time,
		seconds
	FROM app_stats
	UNION
	SELECT
		user_id,
		template_id,
		start_time,
		seconds
	FROM session_stats
)
SELECT
	users.id as user_id,
	users.username,
	users.avatar_url,
	array_agg(DISTINCT template_id)::uuid[] AS template_ids,
	SUM(seconds) AS usage_seconds
FROM combined_stats
JOIN users ON (users.id = combined_stats.user_id)
GROUP BY users.id, username, avatar_url
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

-- name: GetTemplateInsightsByTemplate :many
WITH agent_stats_by_interval_and_user AS (
	SELECT
		date_trunc('minute', was.created_at) AS created_at_trunc,
		was.template_id,
		was.user_id,
		CASE WHEN SUM(was.session_count_vscode) > 0 THEN 60 ELSE 0 END AS usage_vscode_seconds,
		CASE WHEN SUM(was.session_count_jetbrains) > 0 THEN 60 ELSE 0 END AS usage_jetbrains_seconds,
		CASE WHEN SUM(was.session_count_reconnecting_pty) > 0 THEN 60 ELSE 0 END AS usage_reconnecting_pty_seconds,
		CASE WHEN SUM(was.session_count_ssh) > 0 THEN 60 ELSE 0 END AS usage_ssh_seconds
	FROM workspace_agent_stats was
	WHERE
		was.created_at >= @start_time::timestamptz
		AND was.created_at < @end_time::timestamptz
		AND was.connection_count > 0
	GROUP BY created_at_trunc, was.template_id, was.user_id
)

SELECT
	template_id,
	COALESCE(COUNT(DISTINCT user_id))::bigint AS active_users,
	COALESCE(SUM(usage_vscode_seconds), 0)::bigint AS usage_vscode_seconds,
	COALESCE(SUM(usage_jetbrains_seconds), 0)::bigint AS usage_jetbrains_seconds,
	COALESCE(SUM(usage_reconnecting_pty_seconds), 0)::bigint AS usage_reconnecting_pty_seconds,
	COALESCE(SUM(usage_ssh_seconds), 0)::bigint AS usage_ssh_seconds
FROM agent_stats_by_interval_and_user
GROUP BY template_id;

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

-- name: GetTemplateAppInsightsByTemplate :many
WITH app_stats_by_user_and_agent AS (
	SELECT
		s.start_time,
		60 as seconds,
		w.template_id,
		was.user_id,
		was.agent_id,
		was.slug_or_port,
		wa.display_name,
		(wa.slug IS NOT NULL)::boolean AS is_app
	FROM workspace_app_stats was
	JOIN workspaces w ON (
		w.id = was.workspace_id
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
	GROUP BY s.start_time, w.template_id, was.user_id, was.agent_id, was.slug_or_port, wa.display_name, wa.slug
)

SELECT
	template_id,
	display_name,
	slug_or_port,
	COALESCE(COUNT(DISTINCT user_id))::bigint AS active_users,
	SUM(seconds) AS usage_seconds
FROM app_stats_by_user_and_agent
WHERE is_app IS TRUE
GROUP BY template_id, display_name, slug_or_port;

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

-- name: GetTemplateUsageStats :many
SELECT
	*
FROM
	template_usage_stats
WHERE
	start_time >= @start_time::timestamptz
	AND end_time <= @end_time::timestamptz
	AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN template_id = ANY(@template_ids::uuid[]) ELSE TRUE END;

-- name: UpsertTemplateUsageStats :exec
-- This query aggregates the workspace_agent_stats and workspace_app_stats data
-- into a single table for efficient storage and querying. Half-hour buckets are
-- used to store the data, and the minutes are summed for each user and template
-- combination. The result is stored in the template_usage_stats table.
WITH
	latest_start AS (
		SELECT
			COALESCE(
				MAX(start_time) - '1 hour'::interval,
				NOW() - '6 months'::interval
			) AS t
		FROM
			template_usage_stats
	),
	workspace_app_stat_buckets AS (
		SELECT
			-- Truncate the minute to the nearest half hour, this is the bucket size
			-- for the data.
			date_trunc('hour', s.minute_bucket) + trunc(date_part('minute', s.minute_bucket) / 30) * 30 * '1 minute'::interval AS time_bucket,
			w.template_id,
			was.user_id,
			-- Both app stats and agent stats track web terminal usage, but
			-- by different means. The app stats value should be more
			-- accurate so we don't want to discard it just yet.
			CASE
				WHEN was.access_method = 'terminal'
				THEN '[terminal]' -- Unique name, app names can't contain brackets.
				ELSE was.slug_or_port
			END AS app_name,
			COUNT(DISTINCT s.minute_bucket) AS app_minutes,
			-- Store each unique minute bucket for later merge between datasets.
			array_agg(DISTINCT s.minute_bucket) AS minute_buckets
		FROM
			workspace_app_stats AS was
		JOIN
			workspaces AS w
		ON
			w.id = was.workspace_id
		-- Generate a series of minute buckets for each session for computing the
		-- mintes/bucket.
		CROSS JOIN
			generate_series(
				date_trunc('minute', was.session_started_at),
				-- Subtract 1 microsecond to avoid creating an extra series.
				date_trunc('minute', was.session_ended_at - '1 microsecond'::interval),
				'1 minute'::interval
			) AS s(minute_bucket)
		WHERE
			-- s.minute_bucket >= @start_time::timestamptz
			-- AND s.minute_bucket < @end_time::timestamptz
			s.minute_bucket >= (SELECT t FROM latest_start)
			AND s.minute_bucket < NOW()
		GROUP BY
			time_bucket, w.template_id, was.user_id, was.access_method, was.slug_or_port
	),
	agent_stats_buckets AS (
		SELECT
			-- Truncate the minute to the nearest half hour, this is the bucket size
			-- for the data.
			date_trunc('hour', created_at) + trunc(date_part('minute', created_at) / 30) * 30 * '1 minute'::interval AS time_bucket,
			template_id,
			user_id,
			-- Store each unique minute bucket for later merge between datasets.
			array_agg(
				DISTINCT CASE
				WHEN
					session_count_ssh > 0
					-- TODO(mafredri): Enable when we have the column.
					-- OR session_count_sftp > 0
					OR session_count_reconnecting_pty > 0
					OR session_count_vscode > 0
					OR session_count_jetbrains > 0
				THEN
					date_trunc('minute', created_at)
				ELSE
					NULL
				END
			) AS minute_buckets,
			COUNT(DISTINCT CASE WHEN session_count_ssh > 0 THEN date_trunc('minute', created_at) ELSE NULL END) AS ssh_mins,
			-- TODO(mafredri): Enable when we have the column.
			-- COUNT(DISTINCT CASE WHEN session_count_sftp > 0 THEN date_trunc('minute', created_at) ELSE NULL END) AS sftp_mins,
			COUNT(DISTINCT CASE WHEN session_count_reconnecting_pty > 0 THEN date_trunc('minute', created_at) ELSE NULL END) AS reconnecting_pty_mins,
			COUNT(DISTINCT CASE WHEN session_count_vscode > 0 THEN date_trunc('minute', created_at) ELSE NULL END) AS vscode_mins,
			COUNT(DISTINCT CASE WHEN session_count_jetbrains > 0 THEN date_trunc('minute', created_at) ELSE NULL END) AS jetbrains_mins,
			-- NOTE(mafredri): The agent stats are currently very unreliable, and
			-- sometimes the connections are missing, even during active sessions.
			-- Since we can't fully rely on this, we check for "any connection
			-- during this half-hour". A better solution here would be preferable.
			MAX(connection_count) > 0 AS has_connection
		FROM
			workspace_agent_stats
		WHERE
			-- created_at >= @start_time::timestamptz
			-- AND created_at < @end_time::timestamptz
			created_at >= (SELECT t FROM latest_start)
			AND created_at < NOW()
			-- Inclusion criteria to filter out empty results.
			AND (
				session_count_ssh > 0
				-- TODO(mafredri): Enable when we have the column.
				-- OR session_count_sftp > 0
				OR session_count_reconnecting_pty > 0
				OR session_count_vscode > 0
				OR session_count_jetbrains > 0
			)
		GROUP BY
			time_bucket, template_id, user_id
	),
	stats AS (
		SELECT
			stats.time_bucket AS start_time,
			stats.time_bucket + '30 minutes'::interval AS end_time,
			stats.template_id,
			stats.user_id,
			-- Sum/distinct to handle zero/duplicate values due union and to unnest.
			COUNT(DISTINCT minute_bucket) AS usage_mins,
			array_agg(DISTINCT minute_bucket) AS minute_buckets,
			SUM(DISTINCT stats.ssh_mins) AS ssh_mins,
			SUM(DISTINCT stats.sftp_mins) AS sftp_mins,
			SUM(DISTINCT stats.reconnecting_pty_mins) AS reconnecting_pty_mins,
			SUM(DISTINCT stats.vscode_mins) AS vscode_mins,
			SUM(DISTINCT stats.jetbrains_mins) AS jetbrains_mins,
			-- This is what we unnested, re-nest as json.
			jsonb_object_agg(stats.app_name, stats.app_minutes) FILTER (WHERE stats.app_name IS NOT NULL) AS app_usage_mins
		FROM (
			SELECT
				time_bucket,
				template_id,
				user_id,
				0 AS ssh_mins,
				0 AS sftp_mins,
				0 AS reconnecting_pty_mins,
				0 AS vscode_mins,
				0 AS jetbrains_mins,
				app_name,
				app_minutes,
				minute_buckets
			FROM
				workspace_app_stat_buckets

			UNION ALL

			SELECT
				time_bucket,
				template_id,
				user_id,
				ssh_mins,
				-- TODO(mafredri): Enable when we have the column.
				0 AS sftp_mins,
				reconnecting_pty_mins,
				vscode_mins,
				jetbrains_mins,
				NULL AS app_name,
				NULL AS app_minutes,
				minute_buckets
			FROM
				agent_stats_buckets
			WHERE
				-- See note in the agent_stats_buckets CTE.
				has_connection
		) AS stats, unnest(minute_buckets) AS minute_bucket
		GROUP BY
			stats.time_bucket, stats.template_id, stats.user_id
	),
	minute_buckets AS (
		-- Create distinct minute buckets for user-activity, so we can filter out
		-- irrelevant latencies.
		SELECT DISTINCT ON (stats.start_time, stats.template_id, stats.user_id, minute_bucket)
			stats.start_time,
			stats.template_id,
			stats.user_id,
			minute_bucket
		FROM
			stats, unnest(minute_buckets) AS minute_bucket
	),
	latencies AS (
		-- Select all non-zero latencies for all the minutes that a user used the
		-- workspace in some way.
		SELECT
			mb.start_time,
			mb.template_id,
			mb.user_id,
			-- TODO(mafredri): We're doing medians on medians here, we may want to
			-- improve upon this at some point.
			PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY was.connection_median_latency_ms)::real AS median_latency_ms
		FROM
			minute_buckets AS mb
		JOIN
			workspace_agent_stats AS was
		ON
			date_trunc('minute', was.created_at) = mb.minute_bucket
			AND was.template_id = mb.template_id
			AND was.user_id = mb.user_id
			AND was.connection_median_latency_ms >= 0
		GROUP BY
			mb.start_time, mb.template_id, mb.user_id
	)

INSERT INTO template_usage_stats AS tus (
	start_time,
	end_time,
	template_id,
	user_id,
	usage_mins,
	median_latency_ms,
	ssh_mins,
	sftp_mins,
	reconnecting_pty_mins,
	vscode_mins,
	jetbrains_mins,
	app_usage_mins
) (
	SELECT
		stats.start_time,
		stats.end_time,
		stats.template_id,
		stats.user_id,
		stats.usage_mins,
		latencies.median_latency_ms,
		stats.ssh_mins,
		stats.sftp_mins,
		stats.reconnecting_pty_mins,
		stats.vscode_mins,
		stats.jetbrains_mins,
		stats.app_usage_mins
	FROM
		stats
	LEFT JOIN
		latencies
	ON
		-- The latencies group-by ensures there at most one row.
		latencies.start_time = stats.start_time
		AND latencies.template_id = stats.template_id
		AND latencies.user_id = stats.user_id
)
ON CONFLICT
	(start_time, template_id, user_id)
DO UPDATE
SET
	usage_mins = EXCLUDED.usage_mins,
	median_latency_ms = EXCLUDED.median_latency_ms,
	ssh_mins = EXCLUDED.ssh_mins,
	sftp_mins = EXCLUDED.sftp_mins,
	reconnecting_pty_mins = EXCLUDED.reconnecting_pty_mins,
	vscode_mins = EXCLUDED.vscode_mins,
	jetbrains_mins = EXCLUDED.jetbrains_mins,
	app_usage_mins = EXCLUDED.app_usage_mins
WHERE
	(tus.*) IS DISTINCT FROM (EXCLUDED.*);

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
