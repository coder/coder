-- name: GetUserLatencyInsights :many
-- GetUserLatencyInsights returns the median and 95th percentile connection
-- latency that users have experienced. The result can be filtered on
-- template_ids, meaning only user data from workspaces based on those templates
-- will be included.
SELECT
	tus.user_id,
	u.username,
	u.avatar_url,
	array_agg(DISTINCT tus.template_id)::uuid[] AS template_ids,
	COALESCE((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY tus.median_latency_ms)), -1)::float AS workspace_connection_latency_50,
	COALESCE((PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY tus.median_latency_ms)), -1)::float AS workspace_connection_latency_95
FROM
	template_usage_stats tus
JOIN
	users u
ON
	u.id = tus.user_id
WHERE
	tus.start_time >= @start_time::timestamptz
	AND tus.end_time <= @end_time::timestamptz
	AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN tus.template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
GROUP BY
	tus.user_id, u.username, u.avatar_url
ORDER BY
	tus.user_id ASC;

-- name: GetUserActivityInsights :many
-- GetUserActivityInsights returns the ranking with top active users.
-- The result can be filtered on template_ids, meaning only user data
-- from workspaces based on those templates will be included.
-- Note: The usage_seconds and usage_seconds_cumulative differ only when
-- requesting deployment-wide (or multiple template) data. Cumulative
-- produces a bloated value if a user has used multiple templates
-- simultaneously.
WITH
	deployment_stats AS (
		SELECT
			start_time,
			user_id,
			array_agg(template_id) AS template_ids,
			-- See motivation in GetTemplateInsights for LEAST(SUM(n), 30).
			LEAST(SUM(usage_mins), 30) AS usage_mins
		FROM
			template_usage_stats
		WHERE
			start_time >= @start_time::timestamptz
			AND end_time <= @end_time::timestamptz
			AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
		GROUP BY
			start_time, user_id
	),
	template_ids AS (
		SELECT
			user_id,
			array_agg(DISTINCT template_id) AS ids
		FROM
			deployment_stats, unnest(template_ids) template_id
		GROUP BY
			user_id
	)

SELECT
	ds.user_id,
	u.username,
	u.avatar_url,
	t.ids::uuid[] AS template_ids,
	(SUM(ds.usage_mins) * 60)::bigint AS usage_seconds
FROM
	deployment_stats ds
JOIN
	users u
ON
	u.id = ds.user_id
JOIN
	template_ids t
ON
	ds.user_id = t.user_id
GROUP BY
	ds.user_id, u.username, u.avatar_url, t.ids
ORDER BY
	ds.user_id ASC;

-- name: GetTemplateInsights :one
-- GetTemplateInsights returns the aggregate user-produced usage of all
-- workspaces in a given timeframe. The template IDs, active users, and
-- usage_seconds all reflect any usage in the template, including apps.
--
-- When combining data from multiple templates, we must make a guess at
-- how the user behaved for the 30 minute interval. In this case we make
-- the assumption that if the user used two workspaces for 15 minutes,
-- they did so sequentially, thus we sum the usage up to a maximum of
-- 30 minutes with LEAST(SUM(n), 30).
WITH
	insights AS (
		SELECT
			user_id,
			-- See motivation in GetTemplateInsights for LEAST(SUM(n), 30).
			LEAST(SUM(usage_mins), 30) AS usage_mins,
			LEAST(SUM(ssh_mins), 30) AS ssh_mins,
			LEAST(SUM(sftp_mins), 30) AS sftp_mins,
			LEAST(SUM(reconnecting_pty_mins), 30) AS reconnecting_pty_mins,
			LEAST(SUM(vscode_mins), 30) AS vscode_mins,
			LEAST(SUM(jetbrains_mins), 30) AS jetbrains_mins
		FROM
			template_usage_stats
		WHERE
			start_time >= @start_time::timestamptz
			AND end_time <= @end_time::timestamptz
			AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
		GROUP BY
			start_time, user_id
	),
	templates AS (
		SELECT
			array_agg(DISTINCT template_id) AS template_ids,
			array_agg(DISTINCT template_id) FILTER (WHERE ssh_mins > 0) AS ssh_template_ids,
			array_agg(DISTINCT template_id) FILTER (WHERE sftp_mins > 0) AS sftp_template_ids,
			array_agg(DISTINCT template_id) FILTER (WHERE reconnecting_pty_mins > 0) AS reconnecting_pty_template_ids,
			array_agg(DISTINCT template_id) FILTER (WHERE vscode_mins > 0) AS vscode_template_ids,
			array_agg(DISTINCT template_id) FILTER (WHERE jetbrains_mins > 0) AS jetbrains_template_ids
		FROM
			template_usage_stats
		WHERE
			start_time >= @start_time::timestamptz
			AND end_time <= @end_time::timestamptz
			AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
	)

SELECT
	COALESCE((SELECT template_ids FROM templates), '{}')::uuid[] AS template_ids, -- Includes app usage.
	COALESCE((SELECT ssh_template_ids FROM templates), '{}')::uuid[] AS ssh_template_ids,
	COALESCE((SELECT sftp_template_ids FROM templates), '{}')::uuid[] AS sftp_template_ids,
	COALESCE((SELECT reconnecting_pty_template_ids FROM templates), '{}')::uuid[] AS reconnecting_pty_template_ids,
	COALESCE((SELECT vscode_template_ids FROM templates), '{}')::uuid[] AS vscode_template_ids,
	COALESCE((SELECT jetbrains_template_ids FROM templates), '{}')::uuid[] AS jetbrains_template_ids,
	COALESCE(COUNT(DISTINCT user_id), 0)::bigint AS active_users, -- Includes app usage.
	COALESCE(SUM(usage_mins) * 60, 0)::bigint AS usage_total_seconds, -- Includes app usage.
	COALESCE(SUM(ssh_mins) * 60, 0)::bigint AS usage_ssh_seconds,
	COALESCE(SUM(sftp_mins) * 60, 0)::bigint AS usage_sftp_seconds,
	COALESCE(SUM(reconnecting_pty_mins) * 60, 0)::bigint AS usage_reconnecting_pty_seconds,
	COALESCE(SUM(vscode_mins) * 60, 0)::bigint AS usage_vscode_seconds,
	COALESCE(SUM(jetbrains_mins) * 60, 0)::bigint AS usage_jetbrains_seconds
FROM
	insights;

-- name: GetTemplateInsightsByTemplate :many
-- GetTemplateInsightsByTemplate is used for Prometheus metrics. Keep
-- in sync with GetTemplateInsights and UpsertTemplateUsageStats.
WITH
	-- This CTE is used to truncate agent usage into minute buckets, then
	-- flatten the users agent usage within the template so that usage in
	-- multiple workspaces under one template is only counted once for
	-- every minute (per user).
	insights AS (
		SELECT
			template_id,
			user_id,
			COUNT(DISTINCT CASE WHEN session_count_ssh > 0 THEN date_trunc('minute', created_at) ELSE NULL END) AS ssh_mins,
			-- TODO(mafredri): Enable when we have the column.
			-- COUNT(DISTINCT CASE WHEN session_count_sftp > 0 THEN date_trunc('minute', created_at) ELSE NULL END) AS sftp_mins,
			COUNT(DISTINCT CASE WHEN session_count_reconnecting_pty > 0 THEN date_trunc('minute', created_at) ELSE NULL END) AS reconnecting_pty_mins,
			COUNT(DISTINCT CASE WHEN session_count_vscode > 0 THEN date_trunc('minute', created_at) ELSE NULL END) AS vscode_mins,
			COUNT(DISTINCT CASE WHEN session_count_jetbrains > 0 THEN date_trunc('minute', created_at) ELSE NULL END) AS jetbrains_mins,
			-- NOTE(mafredri): The agent stats are currently very unreliable, and
			-- sometimes the connections are missing, even during active sessions.
			-- Since we can't fully rely on this, we check for "any connection
			-- within this bucket". A better solution here would be preferable.
			MAX(connection_count) > 0 AS has_connection
		FROM
			workspace_agent_stats
		WHERE
			created_at >= @start_time::timestamptz
			AND created_at < @end_time::timestamptz
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
			template_id, user_id
	)

SELECT
	template_id,
	COUNT(DISTINCT user_id)::bigint AS active_users,
	(SUM(vscode_mins) * 60)::bigint AS usage_vscode_seconds,
	(SUM(jetbrains_mins) * 60)::bigint AS usage_jetbrains_seconds,
	(SUM(reconnecting_pty_mins) * 60)::bigint AS usage_reconnecting_pty_seconds,
	(SUM(ssh_mins) * 60)::bigint AS usage_ssh_seconds
FROM
	insights
WHERE
	has_connection
GROUP BY
	template_id;

-- name: GetTemplateAppInsights :many
-- GetTemplateAppInsights returns the aggregate usage of each app in a given
-- timeframe. The result can be filtered on template_ids, meaning only user data
-- from workspaces based on those templates will be included.
WITH
	-- Create a list of all unique apps by template, this is used to
	-- filter out irrelevant template usage stats.
	apps AS (
		SELECT DISTINCT ON (ws.template_id, app.slug)
			ws.template_id,
			app.slug,
			app.display_name,
			app.icon
		FROM
			workspaces ws
		JOIN
			workspace_builds AS build
		ON
			build.workspace_id = ws.id
		JOIN
			workspace_resources AS resource
		ON
			resource.job_id = build.job_id
		JOIN
			workspace_agents AS agent
		ON
			agent.resource_id = resource.id
		JOIN
			workspace_apps AS app
		ON
			app.agent_id = agent.id
		WHERE
			-- Partial query parameter filter.
			CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN ws.template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
		ORDER BY
			ws.template_id, app.slug, app.created_at DESC
	),
	-- Join apps and template usage stats to filter out irrelevant rows.
	-- Note that this way of joining will eliminate all data-points that
	-- aren't for "real" apps. That means ports are ignored (even though
	-- they're part of the dataset), as well as are "[terminal]" entries
	-- which are alternate datapoints for reconnecting pty usage.
	template_usage_stats_with_apps AS (
		SELECT
			tus.start_time,
			tus.template_id,
			tus.user_id,
			apps.slug,
			apps.display_name,
			apps.icon,
			tus.app_usage_mins
		FROM
			apps
		JOIN
			template_usage_stats AS tus
		ON
			-- Query parameter filter.
			tus.start_time >= @start_time::timestamptz
			AND tus.end_time <= @end_time::timestamptz
			AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN tus.template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
			-- Primary join condition.
			AND tus.template_id = apps.template_id
			AND tus.app_usage_mins ? apps.slug -- Key exists in object.
	),
	-- Group the app insights by interval, user and unique app. This
	-- allows us to deduplicate a user using the same app across
	-- multiple templates.
	app_insights AS (
		SELECT
			user_id,
			slug,
			display_name,
			icon,
			-- See motivation in GetTemplateInsights for LEAST(SUM(n), 30).
			LEAST(SUM(app_usage.value::smallint), 30) AS usage_mins
		FROM
			template_usage_stats_with_apps, jsonb_each(app_usage_mins) AS app_usage
		WHERE
			app_usage.key = slug
		GROUP BY
			start_time, user_id, slug, display_name, icon
	),
	-- Even though we allow identical apps to be aggregated across
	-- templates, we still want to be able to report which templates
	-- the data comes from.
	templates AS (
		SELECT
			slug,
			display_name,
			icon,
			array_agg(DISTINCT template_id)::uuid[] AS template_ids
		FROM
			template_usage_stats_with_apps
		GROUP BY
			slug, display_name, icon
	)

SELECT
	t.template_ids,
	COUNT(DISTINCT ai.user_id) AS active_users,
	ai.slug,
	ai.display_name,
	ai.icon,
	(SUM(ai.usage_mins) * 60)::bigint AS usage_seconds
FROM
	app_insights AS ai
JOIN
	templates AS t
ON
	t.slug = ai.slug
	AND t.display_name = ai.display_name
	AND t.icon = ai.icon
GROUP BY
	t.template_ids, ai.slug, ai.display_name, ai.icon;

-- name: GetTemplateAppInsightsByTemplate :many
-- GetTemplateAppInsightsByTemplate is used for Prometheus metrics. Keep
-- in sync with GetTemplateAppInsights and UpsertTemplateUsageStats.
WITH
	-- This CTE is used to explode app usage into minute buckets, then
	-- flatten the users app usage within the template so that usage in
	-- multiple workspaces under one template is only counted once for
	-- every minute.
	app_insights AS (
		SELECT
			w.template_id,
			was.user_id,
			-- Both app stats and agent stats track web terminal usage, but
			-- by different means. The app stats value should be more
			-- accurate so we don't want to discard it just yet.
			CASE
				WHEN was.access_method = 'terminal'
				THEN '[terminal]' -- Unique name, app names can't contain brackets.
				ELSE was.slug_or_port
			END::text AS app_name,
			COALESCE(wa.display_name, '') AS display_name,
			(wa.slug IS NOT NULL)::boolean AS is_app,
			COUNT(DISTINCT s.minute_bucket) AS app_minutes
		FROM
			workspace_app_stats AS was
		JOIN
			workspaces AS w
		ON
			w.id = was.workspace_id
		-- We do a left join here because we want to include user IDs that have used
		-- e.g. ports when counting active users.
		LEFT JOIN
			workspace_apps wa
		ON
			wa.agent_id = was.agent_id
			AND wa.slug = was.slug_or_port
		-- Generate a series of minute buckets for each session for computing the
		-- mintes/bucket.
		CROSS JOIN
			generate_series(
				date_trunc('minute', was.session_started_at),
				-- Subtract 1 μs to avoid creating an extra series.
				date_trunc('minute', was.session_ended_at - '1 microsecond'::interval),
				'1 minute'::interval
			) AS s(minute_bucket)
		WHERE
			s.minute_bucket >= @start_time::timestamptz
			AND s.minute_bucket < @end_time::timestamptz
		GROUP BY
			w.template_id, was.user_id, was.access_method, was.slug_or_port, wa.display_name, wa.slug
	)

SELECT
	template_id,
	app_name AS slug_or_port,
	display_name AS display_name,
	COUNT(DISTINCT user_id)::bigint AS active_users,
	(SUM(app_minutes) * 60)::bigint AS usage_seconds
FROM
	app_insights
WHERE
	is_app IS TRUE
GROUP BY
	template_id, slug_or_port, display_name;


-- name: GetTemplateInsightsByInterval :many
-- GetTemplateInsightsByInterval returns all intervals between start and end
-- time, if end time is a partial interval, it will be included in the results and
-- that interval will be shorter than a full one. If there is no data for a selected
-- interval/template, it will be included in the results with 0 active users.
WITH
	ts AS (
		SELECT
			d::timestamptz AS from_,
			LEAST(
				(d::timestamptz + (@interval_days::int || ' day')::interval)::timestamptz,
				@end_time::timestamptz
			)::timestamptz AS to_
		FROM
			generate_series(
				@start_time::timestamptz,
				-- Subtract 1 μs to avoid creating an extra series.
				(@end_time::timestamptz) - '1 microsecond'::interval,
				(@interval_days::int || ' day')::interval
			) AS d
	)

SELECT
	ts.from_ AS start_time,
	ts.to_ AS end_time,
	array_remove(array_agg(DISTINCT tus.template_id), NULL)::uuid[] AS template_ids,
	COUNT(DISTINCT tus.user_id) AS active_users
FROM
	ts
LEFT JOIN
	template_usage_stats AS tus
ON
	tus.start_time >= ts.from_
	AND tus.start_time < ts.to_ -- End time exclusion criteria optimization for index.
	AND tus.end_time <= ts.to_
	AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN tus.template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
GROUP BY
	ts.from_, ts.to_;

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
			-- Truncate to hour so that we always look at even ranges of data.
			date_trunc('hour', COALESCE(
				MAX(start_time) - '1 hour'::interval,
				-- Fallback when there are no template usage stats yet.
				-- App stats can exist before this, but not agent stats,
				-- limit the lookback to avoid inconsistency.
				(SELECT MIN(created_at) FROM workspace_agent_stats)
			)) AS t
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
				-- Subtract 1 μs to avoid creating an extra series.
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
			was.created_at >= (SELECT t FROM latest_start)
			AND was.created_at < NOW()
			AND date_trunc('minute', was.created_at) = mb.minute_bucket
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
