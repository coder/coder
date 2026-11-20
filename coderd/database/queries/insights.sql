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
	-- One pass over the window: per user capped minutes per bucket, and the
	-- user's templates. GROUPING tells the two row kinds apart.
	base AS (
		SELECT
			GROUPING(start_time) = 0 AS is_bucket_row,
			user_id,
			template_id,
			-- See motivation in GetTemplateInsights for LEAST(SUM(n), 30).
			LEAST(SUM(usage_mins), 30) AS usage_mins
		FROM
			template_usage_stats
		WHERE
			start_time >= @start_time::timestamptz
			AND end_time <= @end_time::timestamptz
			AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
		GROUP BY GROUPING SETS ((start_time, user_id), (user_id, template_id))
	),
	user_stats AS (
		SELECT
			user_id,
			array_agg(template_id ORDER BY template_id) FILTER (WHERE NOT is_bucket_row) AS template_ids,
			SUM(usage_mins) FILTER (WHERE is_bucket_row) AS usage_mins
		FROM
			base
		GROUP BY
			user_id
	)

SELECT
	us.user_id,
	u.username,
	u.avatar_url,
	us.template_ids::uuid[] AS template_ids,
	(us.usage_mins * 60)::bigint AS usage_seconds
FROM
	user_stats AS us
JOIN
	users AS u
ON
	u.id = us.user_id
ORDER BY
	us.user_id ASC;

-- name: GetTemplateInsights :one
-- GetTemplateInsights aggregates workspace usage, including apps.
-- Session apps are reported individually; excess unregistered apps fold into
-- overflow. Registered apps and accounting rows do not count toward the limit.
--
-- Per-user minutes are summed across templates and capped at 30 per half hour,
-- assuming workspaces were used sequentially rather than concurrently.
WITH
	base AS (
		-- One pass over the window: per-user capped minutes and the template
		-- list. GROUPING tells the two row kinds apart.
		SELECT
			GROUPING(template_id) = 1 AS is_user_row,
			start_time,
			user_id,
			template_id,
			COUNT(*) AS template_count,
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
			template_count,
			usage_mins
		FROM
			base
		WHERE
			is_user_row
	),
	multi_template_buckets AS (
		-- An app's minutes cap per user per half hour, across templates. Only
		-- these buckets can reach the cap, and most deployments have few, so
		-- the capped grouping below runs on them alone.
		SELECT
			start_time,
			user_id
		FROM
			users
		WHERE
			template_count > 1
	),
	app_usage_by_template AS (
		-- A single row cannot exceed the cap, so these need no per-user
		-- grouping. FILTER, not WHERE: the excluded buckets' minutes belong to
		-- app_usage_capped, but their templates still belong in the list.
		SELECT
			sessions.app_name,
			sessions.template_id,
			SUM(sessions.usage_mins) FILTER (
				WHERE (sessions.start_time, sessions.user_id) NOT IN (SELECT start_time, user_id FROM multi_template_buckets)
			) AS usage_mins
		FROM
			template_usage_stats_session_apps AS sessions
		WHERE
			sessions.start_time >= @start_time::timestamptz
			-- The child table has no end_time, hence the bucket width. Keep
			-- start_time bare so the range stays index-usable.
			AND sessions.start_time <= (@end_time::timestamptz) - '30 minutes'::interval
			AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN sessions.template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
		GROUP BY
			sessions.app_name, sessions.template_id
	),
	app_usage_capped AS (
		SELECT
			sessions.app_name,
			LEAST(SUM(sessions.usage_mins), 30) AS usage_mins
		FROM
			template_usage_stats_session_apps AS sessions
		WHERE
			sessions.start_time >= @start_time::timestamptz
			AND sessions.start_time <= (@end_time::timestamptz) - '30 minutes'::interval
			AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN sessions.template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
			AND EXISTS (
				SELECT 1
				FROM multi_template_buckets AS buckets
				WHERE buckets.start_time = sessions.start_time
					AND buckets.user_id = sessions.user_id
			)
		GROUP BY
			sessions.start_time, sessions.user_id, sessions.app_name
	),
	app_usage AS (
		SELECT
			app_name,
			(SUM(usage_mins) * 60)::bigint AS usage_seconds
		FROM (
			SELECT app_name, usage_mins FROM app_usage_by_template
			UNION ALL
			SELECT app_name, usage_mins FROM app_usage_capped
		) AS parts
		GROUP BY
			app_name
	),
	app_templates AS (
		SELECT
			app_name,
			array_agg(DISTINCT template_id) AS template_ids
		FROM
			app_usage_by_template
		GROUP BY
			app_name
	),
	folded_apps AS (
		SELECT
			app_name
		FROM
			app_usage
		WHERE
			NOT (app_name = ANY(COALESCE(@registered_app_names::text[], '{}'::text[])))
			AND app_name NOT IN ('unknown', 'overflow')
		ORDER BY
			usage_seconds DESC, app_name COLLATE "C"
		OFFSET GREATEST(@max_unregistered_apps::bigint, 0)
	),
	-- The overflow row is only rebuilt when something folds into it.
	folded_names AS (
		SELECT app_name FROM folded_apps
		UNION ALL
		SELECT 'overflow' WHERE EXISTS (SELECT 1 FROM folded_apps)
	),
	-- Overlap between folded apps is not stored. Cap their summed minutes at
	-- each user's active time to bound overflow, and skip the scan if none fold.
	folded_minutes AS (
		SELECT
			sessions.start_time,
			sessions.user_id,
			SUM(sessions.usage_mins) AS usage_mins
		FROM
			template_usage_stats_session_apps AS sessions
		WHERE
			EXISTS (SELECT 1 FROM folded_apps)
			AND sessions.start_time >= @start_time::timestamptz
			AND sessions.start_time <= (@end_time::timestamptz) - '30 minutes'::interval
			AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN sessions.template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
			AND sessions.app_name IN (SELECT app_name FROM folded_names)
		GROUP BY
			sessions.start_time, sessions.user_id
	),
	response_apps AS (
		SELECT
			app_usage.app_name,
			app_usage.usage_seconds,
			app_templates.template_ids
		FROM
			app_usage
		JOIN
			app_templates USING (app_name)
		WHERE
			app_usage.app_name NOT IN (SELECT app_name FROM folded_names)
		UNION ALL
		SELECT
			'overflow',
			(
				SELECT (SUM(LEAST(folded.usage_mins, users.usage_mins)) * 60)::bigint
				FROM folded_minutes AS folded
				JOIN users
				ON users.start_time = folded.start_time
					AND users.user_id = folded.user_id
			),
			(
				SELECT array_agg(DISTINCT template_id)
				FROM app_usage_by_template
				WHERE app_name IN (SELECT app_name FROM folded_names)
			)
		WHERE
			EXISTS (SELECT 1 FROM folded_apps)
	)

SELECT
	COALESCE((SELECT array_agg(DISTINCT template_id) FROM base WHERE NOT is_user_row), '{}')::uuid[] AS template_ids, -- Includes app usage.
	COALESCE(COUNT(DISTINCT user_id), 0)::bigint AS active_users, -- Includes app usage.
	COALESCE(SUM(usage_mins) * 60, 0)::bigint AS usage_total_seconds, -- Includes app usage.
	COALESCE((SELECT jsonb_object_agg(app_name, usage_seconds) FILTER (WHERE usage_seconds IS NOT NULL) FROM response_apps), '{}'::jsonb)::jsonb AS session_app_usage_seconds,
	COALESCE((SELECT jsonb_object_agg(app_name, template_ids) FILTER (WHERE template_ids IS NOT NULL) FROM response_apps), '{}'::jsonb)::jsonb AS session_app_template_ids,
	(SELECT COUNT(*) FROM folded_apps)::bigint AS session_folded_app_count
FROM
	users;

-- name: GetTemplateInsightsByTemplate :many
-- GetTemplateInsightsByTemplate is used for Prometheus metrics. Keep
-- in sync with GetTemplateInsights and UpsertTemplateUsageStats.
--
-- Session usage is per app name. It reads raw agent stats because rollup
-- buckets are coarser than the metrics window.
WITH
	connected AS (
		-- NOTE(mafredri): connection_count covers one report interval, while
		-- the session counts are a gauge, so an idle session reports none.
		-- Hence "any connection within this bucket", pending a better solution.
		-- Grouped, not a WHERE: one connection anywhere in the window keeps
		-- every minute of that pair. One row per pair, so no fan-out.
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
		-- A minute counts once per app, however many of its sessions were
		-- open, which COUNT(DISTINCT) does in the grouping. Deduplicating the
		-- expanded rows first instead spills to disk once a deployment has a
		-- few thousand agents.
		SELECT
			was.template_id,
			was.user_id,
			app_name,
			COUNT(DISTINCT date_trunc('minute', was.created_at)) AS usage_mins
		FROM
			workspace_agent_stats AS was
		CROSS JOIN
			jsonb_object_keys(was.session_counts) AS app_name
		JOIN
			connected AS c
		ON
			c.template_id = was.template_id
			AND c.user_id = was.user_id
		WHERE
			was.created_at >= @start_time::timestamptz
			AND was.created_at < @end_time::timestamptz
			AND was.session_counts <> '{}'::jsonb
		GROUP BY
			was.template_id, was.user_id, app_name
	),
	app_usage AS (
		SELECT
			template_id,
			jsonb_object_agg(app_name, usage_seconds) AS session_app_usage_seconds
		FROM (
			SELECT
				template_id,
				app_name,
				(SUM(usage_mins) * 60)::bigint AS usage_seconds
			FROM
				insights
			GROUP BY
				template_id, app_name
		) AS app_seconds
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
	active_users.template_id,
	active_users.active_users,
	app_usage.session_app_usage_seconds
FROM
	active_users
JOIN
	-- Every counted template has at least one app; both sides come from
	-- insights.
	app_usage
ON
	app_usage.template_id = active_users.template_id;

-- name: GetTemplateAppInsights :many
-- GetTemplateAppInsights returns the aggregate usage of each app in a given
-- timeframe. The result can be filtered on template_ids, meaning only user data
-- from workspaces based on those templates will be included.
--
-- Apps are looked up from the usage names, so the query does not join every
-- app in the deployment to its template.
WITH
	-- (template, app name) pairs with usage in the timeframe.
	usage_slugs AS (
		SELECT DISTINCT
			template_id,
			slug
		FROM (
			SELECT
				tus.template_id,
				usage_slug.slug
			FROM
				template_usage_stats AS tus,
				jsonb_object_keys(tus.app_usage_mins) AS usage_slug(slug)
			WHERE
				tus.start_time >= @start_time::timestamptz
				AND tus.end_time <= @end_time::timestamptz
				AND CASE WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN tus.template_id = ANY(@template_ids::uuid[]) ELSE TRUE END
				-- The rollup only writes objects; this keeps a stray value from
				-- failing jsonb_object_keys.
				AND jsonb_typeof(tus.app_usage_mins) = 'object'
			-- OFFSET 0 keeps the planner from assuming 100 names per row. That
			-- overestimate is multiplied into the probes below and pushes the
			-- plan cost over the JIT optimization threshold, which alone costs
			-- about a second.
			OFFSET 0
		) AS usage_names
	),
	-- Usage names that are some app's slug. Ports and [terminal] are not, and
	-- this rejects them with one index probe per name instead of a walk of
	-- every build of the template. ORDER BY keeps the probe on the index.
	app_slugs AS (
		SELECT
			names.slug
		FROM
			(SELECT DISTINCT slug FROM usage_slugs) AS names
		WHERE
			-- Port-shaped names are always port traffic: workspaceapps routes
			-- them as ports before looking up slugs, so an app with such a slug
			-- can't be opened. Otherwise one app slugged like a port sends every
			-- other template's usage of that port down the slow walk below.
			-- Keep in sync with strconv.ParseUint(strings.TrimSuffix(name, "s"),
			-- 10, 16) in coderd/workspaceapps/request.go.
			NOT CASE
				WHEN names.slug ~ '^[0-9]+s?$' THEN rtrim(names.slug, 's')::numeric <= 65535
				ELSE FALSE
			END
			AND (
				SELECT
					TRUE
				FROM
					workspace_apps AS app
				WHERE
					app.slug = names.slug
				ORDER BY
					app.created_at
				LIMIT 1
			)
	),
	-- Keep the pairs that are apps of some build of the template, used to
	-- filter out irrelevant template usage stats. Each probe stops at the
	-- first match instead of joining every app in the deployment.
	apps AS (
		SELECT
			usage_slugs.template_id,
			usage_slugs.slug
		FROM
			usage_slugs
		WHERE
			-- CASE fixes the evaluation order, cheapest first.
			CASE
				WHEN usage_slugs.slug NOT IN (SELECT slug FROM app_slugs) THEN FALSE
				ELSE EXISTS (
					SELECT
						1
					FROM (
						-- Only live workspaces are indexed by template, and they
						-- nearly always match, so deleted ones are scanned last.
						SELECT id FROM workspaces WHERE template_id = usage_slugs.template_id AND NOT deleted
						UNION ALL
						SELECT id FROM workspaces WHERE template_id = usage_slugs.template_id AND deleted
					) AS ws
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
						AND app.slug = usage_slugs.slug
				)
			END
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
			start_time,
			user_id,
			slug,
			-- See motivation in GetTemplateInsights for LEAST(SUM(n), 30).
			LEAST(SUM(usage_mins), 30) AS usage_mins
		FROM
			template_usage_stats_with_apps
		GROUP BY
			start_time, user_id, slug
	),
	-- Analyze the users unique app usage across all templates. Count
	-- usage across consecutive intervals as continuous usage.
	user_app_usage AS (
		SELECT
			slug,
			user_id,
			SUM(usage_mins) AS usage_mins,
			COUNT(DISTINCT uniq) AS times_used
		FROM (
			SELECT
				slug,
				user_id,
				usage_mins,
				-- Turn start_time into a unique identifier that identifies a users
				-- continuous app usage. The value of uniq is otherwise garbage.
				-- app_insights has one row per start_time, so row_number()
				-- suffices.
				start_time - (
					row_number() OVER (
						PARTITION BY
							user_id, slug
						ORDER BY
							start_time
					) * '30 minutes'::interval
				) AS uniq
			FROM
				app_insights
		) AS runs
		GROUP BY
			slug, user_id
	),
	-- Even though we allow identical apps to be aggregated across
	-- templates, we still want to be able to report which templates
	-- the data comes from. Every pair in apps has usage, so this needs
	-- no pass over the usage rows.
	templates AS (
		SELECT
			slug,
			array_agg(DISTINCT template_id)::uuid[] AS template_ids
		FROM
			apps
		GROUP BY
			slug
	),
	-- The newest app of each filtered template, built only when the walk
	-- below finds no match among the recent apps.
	filtered_newest_apps AS MATERIALIZED (
		SELECT
			ws.template_id,
			app.slug,
			MAX(app.created_at) AS created_at
		FROM
			workspaces AS ws
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
			COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0
			AND ws.template_id = ANY(@template_ids::uuid[])
		GROUP BY
			ws.template_id, app.slug
	),
	-- Apps merge across templates by slug, so show the newest app's
	-- display name and icon among the templates with usage.
	newest_app_times AS (
		SELECT
			templates.slug,
			templates.template_ids,
			COALESCE(
				-- The newest apps nearly always include one of the templates,
				-- so walk them newest first. Deployment wide the walk is
				-- unbounded. With a template filter it stops after the 1000
				-- newest apps and falls back to the filtered templates' apps,
				-- in case those templates have not been built in a while.
				(
					SELECT
						app.created_at
					FROM
						workspace_apps AS app
					JOIN
						workspace_agents AS agent
					ON
						agent.id = app.agent_id
					JOIN
						workspace_resources AS resource
					ON
						resource.id = agent.resource_id
					JOIN
						workspace_builds AS build
					ON
						build.job_id = resource.job_id
					JOIN
						workspaces AS ws
					ON
						ws.id = build.workspace_id
					WHERE
						app.slug = templates.slug
						AND ws.template_id = ANY(templates.template_ids)
						AND app.created_at >= CASE
							WHEN COALESCE(array_length(@template_ids::uuid[], 1), 0) > 0 THEN (
								SELECT
									COALESCE(MIN(recent.created_at), '-infinity'::timestamptz)
								FROM (
									SELECT
										recent.created_at
									FROM
										workspace_apps AS recent
									WHERE
										recent.slug = templates.slug
									ORDER BY
										recent.created_at DESC
									LIMIT 1000
								) AS recent
							)
							ELSE '-infinity'::timestamptz
						END
					ORDER BY
						app.created_at DESC
					LIMIT 1
				),
				(
					SELECT
						MAX(filtered_newest_apps.created_at)
					FROM
						filtered_newest_apps
					WHERE
						filtered_newest_apps.slug = templates.slug
						AND filtered_newest_apps.template_id = ANY(templates.template_ids)
				)
			) AS created_at
		FROM
			templates
	),
	presentation AS (
		SELECT
			newest_app_times.slug,
			newest_app.display_name,
			newest_app.icon
		FROM
			newest_app_times
		CROSS JOIN LATERAL (
			SELECT
				app.display_name,
				app.icon
			FROM
				workspace_apps AS app
			JOIN
				workspace_agents AS agent
			ON
				agent.id = app.agent_id
			JOIN
				workspace_resources AS resource
			ON
				resource.id = agent.resource_id
			JOIN
				workspace_builds AS build
			ON
				build.job_id = resource.job_id
			JOIN
				workspaces AS ws
			ON
				ws.id = build.workspace_id
			WHERE
				app.slug = newest_app_times.slug
				AND app.created_at = newest_app_times.created_at
				AND ws.template_id = ANY(newest_app_times.template_ids)
			ORDER BY
				ws.template_id
			LIMIT 1
		) AS newest_app
	),
	app_totals AS (
		SELECT
			slug,
			COUNT(*) AS active_users,
			(SUM(usage_mins) * 60)::bigint AS usage_seconds,
			SUM(times_used)::bigint AS times_used
		FROM
			user_app_usage
		GROUP BY
			slug
	)

SELECT
	t.template_ids,
	ai.active_users,
	ai.slug::text AS slug,
	p.display_name,
	p.icon,
	ai.usage_seconds,
	ai.times_used
FROM
	app_totals AS ai
JOIN
	templates AS t
ON
	t.slug = ai.slug
JOIN
	presentation AS p
ON
	p.slug = ai.slug;

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
-- Session usage is stored per app name in the child table, so the main row
-- carries no session columns at all. Every recomputed bucket rewrites its own
-- child rows: app names that disappeared are deleted, the rest are upserted.
-- The keys come from the computed set rather than from the main upsert,
-- because the no-op guard below suppresses main rows whose columns did not
-- change while their session usage still has to be corrected.
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
		-- One filtered pass feeds both groupings below.
		SELECT
			date_trunc('hour', created_at) + trunc(date_part('minute', created_at) / 30) * 30 * '1 minute'::interval AS time_bucket,
			template_id,
			user_id,
			date_trunc('minute', created_at) AS minute_bucket,
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
			-- NOTE(mafredri): connection_count covers one report interval,
			-- while the session counts are a gauge, so an idle session reports
			-- none. Hence "any connection during this half-hour", pending a
			-- better solution.
			MAX(connection_count) > 0 AS has_connection
		FROM
			agent_stats_rows
		GROUP BY
			time_bucket, template_id, user_id
	),
	agent_stats_session_minutes AS (
		-- A minute counts once per app however many of its sessions were open.
		SELECT
			agent_stats.time_bucket,
			agent_stats.template_id,
			agent_stats.user_id,
			app_name,
			COUNT(DISTINCT agent_stats.minute_bucket)::smallint AS usage_mins
		FROM (
			-- The window keeps the rows ordered by bucket, so the grouping
			-- below only sorts within each bucket instead of sorting every
			-- expanded row. It matches has_connection in agent_stats_buckets.
			SELECT
				time_bucket,
				template_id,
				user_id,
				minute_bucket,
				session_counts,
				BOOL_OR(connection_count > 0) OVER (PARTITION BY time_bucket, user_id, template_id) AS has_connection
			FROM
				agent_stats_rows
		) AS agent_stats
		CROSS JOIN
			jsonb_object_keys(agent_stats.session_counts) AS app_name
		WHERE
			-- Same gate as the union below, so an app-stats-only bucket
			-- records no session usage.
			agent_stats.has_connection
		GROUP BY
			agent_stats.time_bucket, agent_stats.user_id, agent_stats.template_id, app_name
	),
	stats AS (
		SELECT
			stats.time_bucket AS start_time,
			stats.time_bucket + '30 minutes'::interval AS end_time,
			stats.template_id,
			stats.user_id,
			-- Distinct to handle duplicate values due union and to unnest.
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
	latencies AS (
		-- Select all non-zero latencies for all the minutes that a user used the
		-- workspace in some way. A stat's bucket follows from its minute, so
		-- matching the bucket and checking the minute against the bucket's
		-- minutes keeps the join on the bucket key.
		SELECT
			stats.start_time,
			stats.template_id,
			stats.user_id,
			-- TODO(mafredri): We're doing medians on medians here, we may want to
			-- improve upon this at some point.
			PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY was.connection_median_latency_ms)::real AS median_latency_ms
		FROM
			workspace_agent_stats AS was
		JOIN
			stats
		ON
			stats.start_time = date_trunc('hour', was.created_at) + trunc(date_part('minute', was.created_at) / 30) * 30 * '1 minute'::interval
			AND stats.template_id = was.template_id
			AND stats.user_id = was.user_id
			AND date_trunc('minute', was.created_at) = ANY(stats.minute_buckets)
		WHERE
			was.created_at >= (SELECT t FROM latest_start)
			AND was.created_at < NOW()
			AND was.connection_median_latency_ms > 0
		GROUP BY
			stats.start_time, stats.template_id, stats.user_id
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
			WHERE
				-- Skip unchanged rows before ON CONFLICT locks and WAL-logs
				-- them. Matches the no-op guard below.
				NOT EXISTS (
					SELECT
						1
					FROM
						template_usage_stats AS existing
					WHERE
						existing.start_time = stats.start_time
						AND existing.template_id = stats.template_id
						AND existing.user_id = stats.user_id
						AND existing.end_time = stats.end_time
						AND existing.usage_mins = stats.usage_mins
						AND existing.median_latency_ms IS NOT DISTINCT FROM latencies.median_latency_ms
						AND existing.app_usage_mins IS NOT DISTINCT FROM stats.app_usage_mins
				)
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
	),
	-- The child writes share this statement, so the foreign keys are checked
	-- once it completes and see the main rows the upsert added; Postgres runs
	-- that upsert whether or not the statement reads it.
	-- Session app rows to write or delete: one hash or merge join of the
	-- computed rows with the stored ones. Unchanged rows are left out, as ON
	-- CONFLICT would lock and WAL-log each of them. The CTEs have no
	-- statistics, so anti joins and NOT IN against them can turn into a
	-- rescan per row; a full join cannot.
	session_apps_diff AS MATERIALIZED (
		SELECT
			COALESCE(minutes.time_bucket, existing.start_time) AS start_time,
			COALESCE(minutes.user_id, existing.user_id) AS user_id,
			COALESCE(minutes.template_id, existing.template_id) AS template_id,
			COALESCE(minutes.app_name, existing.app_name) AS app_name,
			minutes.usage_mins
		FROM
			agent_stats_session_minutes AS minutes
		FULL JOIN (
			SELECT
				start_time,
				user_id,
				template_id,
				app_name,
				usage_mins
			FROM
				template_usage_stats_session_apps
			WHERE
				-- Every recomputed bucket starts here.
				start_time >= (SELECT t FROM latest_start)
		) AS existing
		ON
			existing.start_time = minutes.time_bucket
			AND existing.user_id = minutes.user_id
			AND existing.template_id = minutes.template_id
			AND existing.app_name = minutes.app_name
		WHERE
			minutes.usage_mins IS DISTINCT FROM existing.usage_mins
	),
	-- Every recomputed bucket rewrites its own child rows: app names that
	-- disappeared are deleted. A stored bucket with no agent stats left is
	-- not recomputed, so its rows stay.
	delete_apps AS (
		DELETE FROM
			template_usage_stats_session_apps AS apps
		USING
			session_apps_diff AS diff
		WHERE
			diff.usage_mins IS NULL
			AND apps.start_time = diff.start_time
			AND apps.user_id = diff.user_id
			AND apps.template_id = diff.template_id
			AND apps.app_name = diff.app_name
			AND (diff.start_time, diff.template_id, diff.user_id) IN (
				SELECT time_bucket, template_id, user_id
				FROM agent_stats_buckets
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
		start_time,
		template_id,
		user_id,
		app_name,
		usage_mins
	FROM
		session_apps_diff
	WHERE
		usage_mins IS NOT NULL
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
	-- Each build ID appears at most once per group (one parameter row per version
	-- and name, one latest build per workspace), so unnest cannot double count.
	-- The equality join lets Postgres use the (workspace_build_id, name) index.
CROSS JOIN LATERAL unnest(utp.workspace_build_ids) AS b(id)
JOIN workspace_build_parameters wbp ON (wbp.workspace_build_id = b.id AND wbp.name = utp.name)
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
