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
-- Session usage comes from the family child table exactly as the rollup
-- recorded it, so a family the rollup learns about later is reported without a
-- change here.
--
-- When combining data from multiple templates, we must make a guess at
-- how the user behaved for the 30 minute interval. In this case we make
-- the assumption that if the user used two workspaces for 15 minutes,
-- they did so sequentially, thus we sum the usage up to a maximum of
-- 30 minutes with LEAST(SUM(n), 30).
WITH
	base AS MATERIALIZED (
		-- One pass over the main table answers three questions: how many
		-- templates each user touched in a half hour, that user's capped
		-- minutes, and the list of templates in the window. GROUPING marks
		-- which of the two sets a row belongs to.
		SELECT
			GROUPING(template_id) AS is_user_row,
			start_time,
			user_id,
			template_id,
			COUNT(*) AS templates,
			LEAST(SUM(usage_mins), 30) AS usage_mins
		FROM
			template_usage_stats
		WHERE
			start_time >= @start_time::timestamptz
			AND end_time <= @end_time::timestamptz
			AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
		GROUP BY GROUPING SETS ((start_time, user_id), (template_id))
	),
	users AS (
		SELECT
			start_time,
			user_id,
			templates,
			usage_mins
		FROM
			base
		WHERE
			is_user_row = 1
	),
	multi AS MATERIALIZED (
		-- Only a user who used more than one template in the same half hour
		-- can exceed the 30 minute cap, and there are usually none.
		SELECT
			start_time,
			user_id
		FROM
			users
		WHERE
			templates > 1
	),
	single_family_usage AS (
		-- Everything but the multi-template buckets, which cannot exceed the
		-- cap, so they need no per-user grouping. This also collects the
		-- template list per family, for which the cap is irrelevant.
		SELECT
			sessions.family,
			sessions.template_id,
			SUM(LEAST(sessions.usage_mins, 30)) FILTER (
				WHERE (sessions.start_time, sessions.user_id) NOT IN (SELECT start_time, user_id FROM multi)
			) AS usage_mins
		FROM
			template_usage_stats_session_families AS sessions
		WHERE
			sessions.start_time >= @start_time::timestamptz
			AND sessions.start_time + '30 minutes'::interval <= @end_time::timestamptz
			AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN sessions.template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
			AND sessions.usage_mins > 0
		GROUP BY
			sessions.family, sessions.template_id
	),
	multi_family_usage AS (
		-- See motivation in GetTemplateInsights for LEAST(SUM(n), 30).
		SELECT
			sessions.family,
			LEAST(SUM(sessions.usage_mins), 30) AS usage_mins
		FROM
			template_usage_stats_session_families AS sessions
		WHERE
			sessions.start_time >= @start_time::timestamptz
			AND sessions.start_time + '30 minutes'::interval <= @end_time::timestamptz
			AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN sessions.template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
			AND sessions.usage_mins > 0
			AND EXISTS (
				SELECT 1
				FROM multi
				WHERE multi.start_time = sessions.start_time
					AND multi.user_id = sessions.user_id
			)
		GROUP BY
			sessions.start_time, sessions.user_id, sessions.family
	),
	family_usage AS (
		SELECT
			family,
			(SUM(usage_mins) * 60)::bigint AS usage_seconds
		FROM (
			SELECT
				family,
				COALESCE(SUM(usage_mins), 0) AS usage_mins
			FROM
				single_family_usage
			GROUP BY
				family

			UNION ALL

			SELECT
				family,
				SUM(usage_mins) AS usage_mins
			FROM
				multi_family_usage
			GROUP BY
				family
		) AS parts
		GROUP BY
			family
	),
	family_templates AS (
		SELECT
			family,
			array_agg(DISTINCT template_id) AS template_ids
		FROM
			single_family_usage
		GROUP BY
			family
	)

SELECT
	COALESCE((SELECT array_agg(DISTINCT template_id) FROM base WHERE is_user_row = 0), '{}')::uuid[] AS template_ids, -- Includes app usage.
	COALESCE(COUNT(DISTINCT user_id), 0)::bigint AS active_users, -- Includes app usage.
	COALESCE(SUM(usage_mins) * 60, 0)::bigint AS usage_total_seconds, -- Includes app usage.
	-- Family name to usage seconds, and family name to the templates that saw
	-- the family.
	COALESCE((SELECT jsonb_object_agg(family, usage_seconds) FROM family_usage), '{}'::jsonb)::jsonb AS session_family_usage_seconds,
	COALESCE((SELECT jsonb_object_agg(family, template_ids) FROM family_templates), '{}'::jsonb)::jsonb AS session_family_template_ids
FROM
	users;

-- name: GetTemplateInsightsByTemplate :many
-- GetTemplateInsightsByTemplate is used for Prometheus metrics. Keep
-- in sync with GetTemplateInsights and UpsertTemplateUsageStats.
WITH
	expanded AS (
		-- Each row's app names are expanded once and mapped to their family.
		-- app_families maps app name to family name; an app the registry does
		-- not know is attributed to 'unknown' rather than dropped.
		SELECT
			was.template_id,
			was.user_id,
			date_trunc('minute', was.created_at) AS minute,
			COALESCE(@app_families::jsonb ->> app_name, 'unknown') AS family
		FROM
			workspace_agent_stats AS was, jsonb_object_keys(was.session_counts) AS app_name
		WHERE
			was.created_at >= @start_time::timestamptz
			AND was.created_at < @end_time::timestamptz
			AND was.session_counts <> '{}'::jsonb
	),
	minute_family AS (
		-- Deduplicate activity by template, user, minute, and family, so a
		-- minute with two apps of one family counts once for that family.
		SELECT
			template_id,
			user_id,
			minute,
			family
		FROM
			expanded
		GROUP BY
			template_id, user_id, minute, family
	),
	connected AS (
		-- NOTE(mafredri): The agent stats are currently very unreliable, and
		-- sometimes the connections are missing, even during active sessions.
		-- Since we can't fully rely on this, we check for "any connection
		-- within this bucket". A better solution here would be preferable.
		SELECT
			template_id,
			user_id
		FROM
			workspace_agent_stats
		WHERE
			created_at >= @start_time::timestamptz
			AND created_at < @end_time::timestamptz
			AND session_counts <> '{}'::jsonb
		GROUP BY
			template_id, user_id
		HAVING
			BOOL_OR(connection_count > 0)
	),
	insights AS (
		SELECT
			mf.template_id,
			mf.user_id,
			mf.family,
			COUNT(*) AS usage_mins
		FROM
			minute_family AS mf
		JOIN
			connected AS c
		ON
			c.template_id = mf.template_id
			AND c.user_id = mf.user_id
		GROUP BY
			mf.template_id, mf.user_id, mf.family
	),
	family_usage AS (
		SELECT
			template_id,
			jsonb_object_agg(family, usage_seconds) AS session_family_usage_seconds
		FROM (
			SELECT
				template_id,
				family,
				(SUM(usage_mins) * 60)::bigint AS usage_seconds
			FROM
				insights
			GROUP BY
				template_id, family
		) AS family_seconds
		GROUP BY
			template_id
	),
	active_users AS (
		SELECT
			template_id,
			COUNT(DISTINCT user_id)::bigint AS active_users
		FROM
			insights
		GROUP BY
			template_id
	)

SELECT
	au.template_id,
	au.active_users,
	COALESCE(fu.session_family_usage_seconds, '{}'::jsonb)::jsonb AS session_family_usage_seconds
FROM
	active_users AS au
LEFT JOIN
	family_usage AS fu
ON
	fu.template_id = au.template_id;

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
			(tus.app_usage_mins -> apps.slug)::smallint AS usage_mins
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
			LEAST(SUM(usage_mins), 30) AS usage_mins
		FROM
			template_usage_stats_with_apps
		GROUP BY
			start_time, user_id, slug, display_name, icon
	),
	-- Analyze the users unique app usage across all templates. Count
	-- usage across consecutive intervals as continuous usage.
	times_used AS (
		SELECT DISTINCT ON (user_id, slug, display_name, icon, uniq)
			slug,
			display_name,
			icon,
			-- Turn start_time into a unique identifier that identifies a users
			-- continuous app usage. The value of uniq is otherwise garbage.
			--
			-- Since we're aggregating per user app usage across templates,
			-- there can be duplicate start_times. To handle this, we use the
			-- dense_rank() function, otherwise row_number() would suffice.
			start_time - (
				dense_rank() OVER (
					PARTITION BY
						user_id, slug, display_name, icon
					ORDER BY
						start_time
				) * '30 minutes'::interval
			) AS uniq
		FROM
			template_usage_stats_with_apps
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
	(SUM(ai.usage_mins) * 60)::bigint AS usage_seconds,
	COALESCE((
		SELECT
			COUNT(*)
		FROM
			times_used
		WHERE
			times_used.slug = ai.slug
			AND times_used.display_name = ai.display_name
			AND times_used.icon = ai.icon
	), 0)::bigint AS times_used
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
	filtered_stats AS (
		SELECT
		was.workspace_id,
		was.user_id,
		was.agent_id,
		was.access_method,
		was.slug_or_port,
		was.session_started_at,
		was.session_ended_at
		FROM
			workspace_app_stats AS was
		WHERE
			was.session_ended_at >= @start_time::timestamptz
			AND was.session_started_at <  @end_time::timestamptz
	),
	-- This CTE is used to explode app usage into minute buckets, then
	-- flatten the users app usage within the template so that usage in
	-- multiple workspaces under one template is only counted once for
	-- every minute.
	app_insights AS (
		SELECT
			w.template_id,
			fs.user_id,
			-- Both app stats and agent stats track web terminal usage, but
			-- by different means. The app stats value should be more
			-- accurate so we don't want to discard it just yet.
			CASE
				WHEN fs.access_method = 'terminal'
				THEN '[terminal]' -- Unique name, app names can't contain brackets.
				ELSE fs.slug_or_port
			END::text AS app_name,
			COALESCE(wa.display_name, '') AS display_name,
			(wa.slug IS NOT NULL)::boolean AS is_app,
			COUNT(DISTINCT s.minute_bucket) AS app_minutes
		FROM
			filtered_stats AS fs
		JOIN
			workspaces AS w
		ON
			w.id = fs.workspace_id
		-- We do a left join here because we want to include user IDs that have used
		-- e.g. ports when counting active users.
		LEFT JOIN
			workspace_apps wa
		ON
			wa.agent_id = fs.agent_id
			AND wa.slug = fs.slug_or_port
		-- Generate a series of minute buckets for each session for computing the
		-- mintes/bucket.
		CROSS JOIN
			generate_series(
				date_trunc('minute', fs.session_started_at),
				-- Subtract 1 μs to avoid creating an extra series.
				date_trunc('minute', fs.session_ended_at - '1 microsecond'::interval),
				'1 minute'::interval
			) AS s(minute_bucket)
		WHERE
			s.minute_bucket >= @start_time::timestamptz
			AND s.minute_bucket < @end_time::timestamptz
		GROUP BY
			w.template_id, fs.user_id, fs.access_method, fs.slug_or_port, wa.display_name, wa.slug
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
--
-- Session usage is stored per app name and per app family in the child tables,
-- so the main row carries no session columns at all. Every recomputed bucket
-- rewrites its own child rows: names that disappeared are deleted, the rest
-- are upserted. The keys come from the computed set rather than from the main
-- upsert, because the no-op guard below suppresses main rows whose columns did
-- not change while their session usage still has to be corrected.
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
	filtered_app_stats AS (
        SELECT
            was.workspace_id,
            was.user_id,
            was.agent_id,
            was.access_method,
            was.slug_or_port,
            was.session_started_at,
            was.session_ended_at
        FROM
            workspace_app_stats AS was
        WHERE
            was.session_ended_at >= (SELECT t FROM latest_start)
            AND was.session_started_at < NOW()
    ),
	workspace_app_stat_buckets AS (
		SELECT
			-- Truncate the minute to the nearest half hour, this is the bucket size
			-- for the data.
			date_trunc('hour', s.minute_bucket) + trunc(date_part('minute', s.minute_bucket) / 30) * 30 * '1 minute'::interval AS time_bucket,
			w.template_id,
			fas.user_id,
			-- Both app stats and agent stats track web terminal usage, but
			-- by different means. The app stats value should be more
			-- accurate so we don't want to discard it just yet.
			CASE
				WHEN fas.access_method = 'terminal'
				THEN '[terminal]' -- Unique name, app names can't contain brackets.
				ELSE fas.slug_or_port
			END AS app_name,
			COUNT(DISTINCT s.minute_bucket) AS app_minutes,
			-- Store each unique minute bucket for later merge between datasets.
			array_agg(DISTINCT s.minute_bucket) AS minute_buckets
		FROM
			filtered_app_stats AS fas
		JOIN
			workspaces AS w
		ON
			w.id = fas.workspace_id
		-- Generate a series of minute buckets for each session for computing the
		-- mintes/bucket.
		CROSS JOIN
			generate_series(
				date_trunc('minute', fas.session_started_at),
				-- Subtract 1 μs to avoid creating an extra series.
				date_trunc('minute', fas.session_ended_at - '1 microsecond'::interval),
				'1 minute'::interval
			) AS s(minute_bucket)
		WHERE
			-- s.minute_bucket >= @start_time::timestamptz
			-- AND s.minute_bucket < @end_time::timestamptz
			s.minute_bucket >= (SELECT t FROM latest_start)
			AND s.minute_bucket < NOW()
		GROUP BY
			time_bucket, w.template_id, fas.user_id, fas.access_method, fas.slug_or_port
	),
	agent_stats_rows AS (
		-- One filtered pass over workspace_agent_stats feeds both the bucket
		-- grouping and the per-app mask grouping below, instead of scanning the
		-- table once for each. The minute bit is computed here so the mask
		-- grouping never touches created_at again.
		SELECT
			date_trunc('hour', created_at) + trunc(date_part('minute', created_at) / 30) * 30 * '1 minute'::interval AS time_bucket,
			template_id,
			user_id,
			date_trunc('minute', created_at) AS minute_bucket,
			(1 << (date_part('minute', created_at)::int % 30)) AS minute_bit,
			connection_count,
			session_counts
		FROM
			workspace_agent_stats
		WHERE
			-- created_at >= @start_time::timestamptz
			-- AND created_at < @end_time::timestamptz
			created_at >= (SELECT t FROM latest_start)
			AND created_at < NOW()
			AND session_counts <> '{}'::jsonb
	),
	agent_stats_buckets AS (
		SELECT
			time_bucket,
			template_id,
			user_id,
			-- Store each unique minute bucket for later merge between datasets.
			array_agg(DISTINCT minute_bucket) AS minute_buckets,
			-- NOTE(mafredri): The agent stats are currently very unreliable, and
			-- sometimes the connections are missing, even during active sessions.
			-- Since we can't fully rely on this, we check for "any connection
			-- during this half-hour". A better solution here would be preferable.
			MAX(connection_count) > 0 AS has_connection
		FROM
			agent_stats_rows
		GROUP BY
			time_bucket, template_id, user_id
	),
	app_family_registry AS (
		-- The session count attribution registry, app name to family name.
		SELECT
			app,
			family
		FROM
			jsonb_each_text(@app_families::jsonb) AS registry(app, family)
	),
	agent_stats_app_minutes AS (
		-- One bit per minute of the half-hour bucket, per app name, instead of
		-- a count: masks can be OR'd into a family below without counting a
		-- minute twice when two apps of the same family were active in it.
		-- Transient, only the minute counts derived from them are stored.
		SELECT
			time_bucket,
			template_id,
			user_id,
			app_name,
			bit_or(minute_bit) AS minute_mask
		FROM
			agent_stats_rows, jsonb_object_keys(session_counts) AS app_name
		GROUP BY
			time_bucket, template_id, user_id, app_name
	),
	agent_stats_session_masks AS (
		-- One pass over the per-app masks emits both groupings: the app rows
		-- keep each mask as it is, the family rows OR the masks of every app
		-- in the family. An app name the registry does not know is attributed
		-- to 'unknown' rather than dropped, so a newly reported app still
		-- lands somewhere. GROUPING() marks which set a row came from, because
		-- the app column is null in the family rows.
		SELECT
			time_bucket,
			template_id,
			user_id,
			agent_stats_app_minutes.app_name,
			COALESCE(app_family_registry.family, 'unknown') AS family,
			GROUPING(agent_stats_app_minutes.app_name) AS family_group,
			bit_or(minute_mask) AS minute_mask
		FROM
			agent_stats_app_minutes
		LEFT JOIN
			app_family_registry
		ON
			app_family_registry.app = agent_stats_app_minutes.app_name
		GROUP BY GROUPING SETS (
			(time_bucket, template_id, user_id, agent_stats_app_minutes.app_name),
			(time_bucket, template_id, user_id, COALESCE(app_family_registry.family, 'unknown'))
		)
	),
	agent_stats_session_minutes AS (
		-- The minutes each name was active, counted from the bits set in its
		-- mask. Postgres 13 has no bit_count, so the set bits are counted by
		-- stripping the zeros out of the mask's text form.
		SELECT
			masks.time_bucket,
			masks.template_id,
			masks.user_id,
			masks.family_group,
			CASE WHEN masks.family_group = 0 THEN masks.app_name ELSE masks.family END AS name,
			length(replace(masks.minute_mask::bit(30)::text, '0', ''))::smallint AS usage_mins
		FROM
			agent_stats_session_masks AS masks
		JOIN
			agent_stats_buckets AS buckets
		ON
			buckets.time_bucket = masks.time_bucket
			AND buckets.template_id = masks.template_id
			AND buckets.user_id = masks.user_id
			-- The same gate the union below applies to agent stats, so a
			-- bucket that only has app stats records no session usage.
			AND buckets.has_connection
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
			-- This is what we unnested, re-nest as json.
			jsonb_object_agg(stats.app_name, stats.app_minutes) FILTER (WHERE stats.app_name IS NOT NULL) AS app_usage_mins
		FROM (
			SELECT
				time_bucket,
				template_id,
				user_id,
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
			AND was.connection_median_latency_ms > 0
		GROUP BY
			mb.start_time, mb.template_id, mb.user_id
	),
	upsert_stats AS (
		INSERT INTO template_usage_stats AS tus (
			start_time,
			end_time,
			template_id,
			user_id,
			usage_mins,
			median_latency_ms,
			app_usage_mins
		) (
			SELECT
				stats.start_time,
				stats.end_time,
				stats.template_id,
				stats.user_id,
				stats.usage_mins,
				latencies.median_latency_ms,
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
			app_usage_mins = EXCLUDED.app_usage_mins
		WHERE
			(tus.*) IS DISTINCT FROM (EXCLUDED.*)
		RETURNING
			tus.start_time
	),
	-- The child writes below run in this same statement, so the foreign key
	-- triggers fire once it completes and see the main rows the upsert above
	-- added. The delete and the insert of each pair never touch the same row:
	-- the delete matches names the recomputed bucket no longer has, the insert
	-- only the names it does have.
	--
	-- The deletes test membership with NOT IN rather than NOT EXISTS on purpose.
	-- The planner has no statistics for the recomputed CTE and estimates it at
	-- a few rows, which turns NOT EXISTS into a nested loop that rescans the
	-- CTE per candidate row, measured at 20 seconds per rollup. NOT IN is
	-- planned as a hashed subplan built once, whatever the estimate. Every
	-- column in the subquery is non-null, so the two forms delete the same
	-- rows; the IS NOT NULL guard keeps that true if the CTE ever changes.
	delete_families AS (
		DELETE FROM
			template_usage_stats_session_families AS families
		USING
			stats
		WHERE
			families.start_time = stats.start_time
			AND families.template_id = stats.template_id
			AND families.user_id = stats.user_id
			AND (families.start_time, families.template_id, families.user_id, families.family) NOT IN (
				SELECT time_bucket, template_id, user_id, name
				FROM agent_stats_session_minutes
				WHERE family_group = 1 AND name IS NOT NULL
			)
	),
	upsert_families AS (
		INSERT INTO template_usage_stats_session_families AS families (
			start_time,
			template_id,
			user_id,
			family,
			usage_mins
		) (
			SELECT
				time_bucket,
				template_id,
				user_id,
				name,
				usage_mins
			FROM
				agent_stats_session_minutes
			WHERE
				family_group = 1
		)
		ON CONFLICT
			(start_time, template_id, user_id, family)
		DO UPDATE
		SET
			usage_mins = EXCLUDED.usage_mins
		WHERE
			families.usage_mins IS DISTINCT FROM EXCLUDED.usage_mins
	),
	delete_apps AS (
		DELETE FROM
			template_usage_stats_session_apps AS apps
		USING
			stats
		WHERE
			apps.start_time = stats.start_time
			AND apps.template_id = stats.template_id
			AND apps.user_id = stats.user_id
			AND (apps.start_time, apps.template_id, apps.user_id, apps.app_name) NOT IN (
				SELECT time_bucket, template_id, user_id, name
				FROM agent_stats_session_minutes
				WHERE family_group = 0 AND name IS NOT NULL
			)
	)

INSERT INTO template_usage_stats_session_apps AS apps (
	start_time,
	template_id,
	user_id,
	app_name,
	usage_mins
) (
	SELECT
		time_bucket,
		template_id,
		user_id,
		name,
		usage_mins
	FROM
		agent_stats_session_minutes
	WHERE
		family_group = 0
)
ON CONFLICT
	(start_time, template_id, user_id, app_name)
DO UPDATE
SET
	usage_mins = EXCLUDED.usage_mins
WHERE
	apps.usage_mins IS DISTINCT FROM EXCLUDED.usage_mins;

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

-- name: GetUserStatusCounts :many
-- GetUserStatusCounts returns the count of users in each status over time.
-- The time range is inclusively defined by the start_time and end_time parameters.
WITH
system_users AS (
    SELECT id FROM users WHERE is_system = TRUE
),
	-- dates_of_interest generates the dates that will represent the horizontal axis of the chart.
dates_of_interest AS (
  SELECT timezone(@tz::text, gs_local) AS date
  FROM generate_series(
    timezone(@tz::text, @start_time::timestamptz),
    timezone(@tz::text, @end_time::timestamptz),
    interval '1 day'
  ) AS gs_local
),
	-- latest_status_before_range selects the last status of each user before the start_time.
	-- This represents the status of all users at the start of the time range.
latest_status_before_range AS (
    SELECT
        DISTINCT usc.user_id,
        usc.new_status,
        usc.changed_at
    FROM user_status_changes usc
	LEFT JOIN LATERAL (
		SELECT COUNT(*) > 0 AS deleted
		FROM user_deleted ud
		WHERE ud.user_id = usc.user_id AND (ud.deleted_at < usc.changed_at OR ud.deleted_at < @start_time)
	) AS ud ON true
    WHERE usc.user_id NOT IN (SELECT id FROM system_users)
        AND NOT ud.deleted
        AND usc.changed_at < @start_time::timestamptz
    ORDER BY usc.user_id, usc.changed_at DESC
),
	-- status_changes_during_range selects the statuses of each user during the start_time and end_time.
status_changes_during_range AS (
    SELECT
        usc.user_id,
        usc.new_status,
        usc.changed_at
    FROM user_status_changes usc
	LEFT JOIN LATERAL (
		SELECT COUNT(*) > 0 AS deleted
		FROM user_deleted ud
		WHERE ud.user_id = usc.user_id AND ud.deleted_at < usc.changed_at
	) AS ud ON true
    WHERE usc.user_id NOT IN (SELECT id FROM system_users)
        AND NOT ud.deleted
        AND usc.changed_at >= @start_time::timestamptz
        AND usc.changed_at <= @end_time::timestamptz
),
relevant_status_changes AS (
    SELECT user_id, new_status, changed_at
    FROM latest_status_before_range

    UNION ALL

    SELECT user_id, new_status, changed_at
    FROM status_changes_during_range
),
	-- statuses selects all the distinct statuses that were present just before and during the time range.
	-- Each status will have a series on the chart.
statuses AS (
	SELECT DISTINCT new_status FROM relevant_status_changes
),
	-- ranked_status_change_per_user_per_date selects the latest status change for each user on each date.
	-- The last status for a user on every given date will be counted.
ranked_status_change_per_user_per_date AS (
	SELECT
	d.date,
	rsc1.user_id,
	ROW_NUMBER() OVER (PARTITION BY d.date, rsc1.user_id ORDER BY rsc1.changed_at DESC) AS rn,
	rsc1.new_status
	FROM dates_of_interest d
	LEFT JOIN relevant_status_changes rsc1 ON rsc1.changed_at <= d.date
)
SELECT
	rscpupd.date::timestamptz AS date,
	statuses.new_status AS status,
	COUNT(rscpupd.user_id) FILTER (
		WHERE rscpupd.rn = 1
		AND (
			rscpupd.new_status = statuses.new_status
			AND (
				-- Include users who haven't been deleted
				NOT EXISTS (SELECT 1 FROM user_deleted WHERE user_id = rscpupd.user_id)
				OR
				-- Or users whose deletion date is after the current date we're looking at
				rscpupd.date < (SELECT deleted_at FROM user_deleted WHERE user_id = rscpupd.user_id)
			)
		)
	) AS count
FROM ranked_status_change_per_user_per_date rscpupd
CROSS JOIN statuses
GROUP BY rscpupd.date, statuses.new_status
ORDER BY rscpupd.date;
