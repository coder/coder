-- name: InsertWorkspaceAgentStats :exec
INSERT INTO
	workspace_agent_stats (
		id,
		created_at,
		user_id,
		workspace_id,
		template_id,
		agent_id,
		connections_by_proto,
		connection_count,
		rx_packets,
		rx_bytes,
		tx_packets,
		tx_bytes,
		session_counts,
		connection_median_latency_ms,
		usage
	)
SELECT
	unnest(@id :: uuid[]) AS id,
	unnest(@created_at :: timestamptz[]) AS created_at,
	unnest(@user_id :: uuid[]) AS user_id,
	unnest(@workspace_id :: uuid[]) AS workspace_id,
	unnest(@template_id :: uuid[]) AS template_id,
	unnest(@agent_id :: uuid[]) AS agent_id,
	jsonb_array_elements(@connections_by_proto :: jsonb) AS connections_by_proto,
	unnest(@connection_count :: bigint[]) AS connection_count,
	unnest(@rx_packets :: bigint[]) AS rx_packets,
	unnest(@rx_bytes :: bigint[]) AS rx_bytes,
	unnest(@tx_packets :: bigint[]) AS tx_packets,
	unnest(@tx_bytes :: bigint[]) AS tx_bytes,
	jsonb_array_elements(@session_counts :: jsonb) AS session_counts,
	unnest(@connection_median_latency_ms :: double precision[]) AS connection_median_latency_ms,
	unnest(@usage :: boolean[]) AS usage;

-- name: DeleteOldWorkspaceAgentStats :exec
DELETE FROM
	workspace_agent_stats
WHERE
	created_at < (
		SELECT
			COALESCE(
				-- When generating initial template usage stats, all the
				-- raw agent stats are needed, after that only ~30 mins
				-- from last rollup is needed. Deployment stats seem to
				-- use between 15 mins and 1 hour of data. We keep a
				-- little bit more (1 day) just in case.
				MAX(start_time) - '1 days'::interval,
				-- Fall back to ~6 months ago if there are no template
				-- usage stats so that we don't delete the data before
				-- it's rolled up.
				NOW() - '180 days'::interval
			)
		FROM
			template_usage_stats
	)
	AND created_at < (
		-- Delete at most in batches of 4 hours (with this batch size, assuming
		-- 1 iteration / 10 minutes, we can clear out the previous 6 months of
		-- data in 7.5 days) whilst keeping the DB load low.
		SELECT
			COALESCE(MIN(created_at) + '4 hours'::interval, NOW())
		FROM
			workspace_agent_stats
	);

-- name: GetDeploymentWorkspaceAgentStats :one
-- The session count sum runs in its own subquery: decomposing session_counts
-- in the FROM clause would emit one row per app name and multiply the byte and
-- latency aggregates below. Summing per app name and folding the names into
-- families in Go keeps a session reported under a new name counted.
WITH stats AS (
	SELECT
		agent_id,
		created_at,
		rx_bytes,
		tx_bytes,
		connection_median_latency_ms,
		session_counts,
		ROW_NUMBER() OVER (PARTITION BY agent_id ORDER BY created_at DESC) AS rn
	FROM workspace_agent_stats
	WHERE created_at > $1
)
SELECT
	coalesce(SUM(rx_bytes), 0)::bigint AS workspace_rx_bytes,
	coalesce(SUM(tx_bytes), 0)::bigint AS workspace_tx_bytes,
	-- The greater than 0 is to support legacy agents that don't report connection_median_latency_ms.
	coalesce((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY connection_median_latency_ms) FILTER (WHERE connection_median_latency_ms > 0)), -1)::FLOAT AS workspace_connection_latency_50,
	coalesce((PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY connection_median_latency_ms) FILTER (WHERE connection_median_latency_ms > 0)), -1)::FLOAT AS workspace_connection_latency_95,
	coalesce((
		SELECT
			jsonb_object_agg(app_name, app_sessions)
		FROM (
			SELECT
				sess.app_name,
				SUM(sess.sessions::bigint) AS app_sessions
			FROM stats, jsonb_each_text(stats.session_counts) AS sess(app_name, sessions)
			-- Only the latest row per agent holds current sessions.
			WHERE stats.rn = 1
			GROUP BY sess.app_name
		) AS app_totals
	), '{}'::jsonb)::jsonb AS session_counts
FROM stats;

-- name: GetDeploymentWorkspaceAgentUsageStats :one
WITH agent_stats AS (
	SELECT
		coalesce(SUM(rx_bytes), 0)::bigint AS workspace_rx_bytes,
		coalesce(SUM(tx_bytes), 0)::bigint AS workspace_tx_bytes,
		coalesce((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY connection_median_latency_ms)), -1)::FLOAT AS workspace_connection_latency_50,
		coalesce((PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY connection_median_latency_ms)), -1)::FLOAT AS workspace_connection_latency_95
	 FROM workspace_agent_stats
	 	-- The greater than 0 is to support legacy agents that don't report connection_median_latency_ms.
		WHERE workspace_agent_stats.created_at > $1 AND connection_median_latency_ms > 0
),
latest_minutes AS (
	SELECT
		agent_id,
		MAX(date_trunc('minute', created_at)) AS minute_bucket
	FROM
		workspace_agent_stats
	WHERE
		created_at >= $1
		-- Exclude the current partial minute.
		AND created_at < date_trunc('minute', now())
		AND usage
	GROUP BY
		agent_id
),
latest_agent_stats AS (
	-- Aggregating the per app name sums separately keeps the byte and latency
	-- aggregates in agent_stats free of the decomposed rows.
	SELECT
		coalesce(jsonb_object_agg(app_name, app_sessions), '{}'::jsonb)::jsonb AS session_counts
	FROM (
		SELECT
			sess.app_name,
			SUM(sess.sessions::bigint) AS app_sessions
		FROM
			latest_minutes
		JOIN
			workspace_agent_stats AS stats
		ON
			stats.agent_id = latest_minutes.agent_id
			AND stats.created_at >= $1
			AND stats.created_at >= latest_minutes.minute_bucket
			AND stats.created_at < latest_minutes.minute_bucket + '1 minute'::interval
			AND stats.usage,
			jsonb_each_text(stats.session_counts) AS sess(app_name, sessions)
		GROUP BY sess.app_name
	) AS app_totals
)
SELECT * FROM agent_stats, latest_agent_stats;

-- name: GetWorkspaceAgentStats :many
WITH agent_stats AS (
	SELECT
		user_id,
		agent_id,
		workspace_id,
		template_id,
		MIN(created_at)::timestamptz AS aggregated_from,
		coalesce(SUM(rx_bytes), 0)::bigint AS workspace_rx_bytes,
		coalesce(SUM(tx_bytes), 0)::bigint AS workspace_tx_bytes,
		coalesce((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY connection_median_latency_ms)), -1)::FLOAT AS workspace_connection_latency_50,
		coalesce((PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY connection_median_latency_ms)), -1)::FLOAT AS workspace_connection_latency_95
	 FROM workspace_agent_stats
	-- The greater than 0 is to support legacy agents that don't report connection_median_latency_ms.
	WHERE workspace_agent_stats.created_at > $1 AND connection_median_latency_ms > 0
	GROUP BY user_id, agent_id, workspace_id, template_id
), latest_stats AS (
	SELECT *, ROW_NUMBER() OVER(PARTITION BY agent_id ORDER BY created_at DESC) AS rn
	FROM workspace_agent_stats WHERE created_at > $1
), latest_sessions AS (
	-- Two levels: the inner query sums each app name on the latest row per
	-- agent, the outer one folds those sums back into a single jsonb object.
	-- rn = 1 leaves one row per agent, so grouping by agent_id alone matches
	-- the per-agent grouping of agent_stats above.
	SELECT
		agent_id,
		coalesce(jsonb_object_agg(app_name, app_sessions), '{}'::jsonb)::jsonb AS session_counts
	FROM (
		SELECT
			a.agent_id,
			sess.app_name,
			SUM(sess.sessions::bigint) AS app_sessions
		FROM latest_stats AS a, jsonb_each_text(a.session_counts) AS sess(app_name, sessions)
		WHERE a.rn = 1
		GROUP BY a.agent_id, sess.app_name
	) AS app_totals
	GROUP BY agent_id
), latest_agent_stats AS (
	-- Joined rather than selected from latest_sessions so an agent whose
	-- latest row reports no sessions at all keeps its row here.
	SELECT
		a.agent_id,
		coalesce(latest_sessions.session_counts, '{}'::jsonb)::jsonb AS session_counts
	FROM latest_stats AS a
	LEFT JOIN latest_sessions ON latest_sessions.agent_id = a.agent_id
	WHERE a.rn = 1
	GROUP BY a.user_id, a.agent_id, a.workspace_id, a.template_id, latest_sessions.session_counts
)
SELECT * FROM agent_stats JOIN latest_agent_stats ON agent_stats.agent_id = latest_agent_stats.agent_id;

-- name: GetWorkspaceAgentUsageStats :many
WITH stats AS (
	SELECT
		*,
		-- The greater than 0 is to support legacy agents that don't report connection_median_latency_ms.
		created_at > $1 AND connection_median_latency_ms > 0 AS reports_latency,
		usage AND date_trunc('minute', created_at) = MAX(date_trunc('minute', created_at)) FILTER (
			-- Exclude the current partial minute.
			WHERE usage AND created_at < date_trunc('minute', now())
		) OVER (PARTITION BY agent_id) AS in_latest_usage_minute
	FROM workspace_agent_stats
	WHERE created_at >= $1
), latest_sessions AS (
	-- One row per agent, so joining it below neither multiplies the byte and
	-- latency aggregates nor adds groups.
	SELECT
		agent_id,
		coalesce(jsonb_object_agg(app_name, app_sessions), '{}'::jsonb)::jsonb AS session_counts
	FROM (
		SELECT
			stats.agent_id,
			sess.app_name,
			SUM(sess.sessions::bigint) AS app_sessions
		FROM stats, jsonb_each_text(stats.session_counts) AS sess(app_name, sessions)
		WHERE stats.in_latest_usage_minute
		GROUP BY stats.agent_id, sess.app_name
	) AS app_totals
	GROUP BY agent_id
)
SELECT
	stats.user_id,
	stats.agent_id,
	stats.workspace_id,
	stats.template_id,
	MIN(stats.created_at) FILTER (WHERE reports_latency)::timestamptz AS aggregated_from,
	coalesce(SUM(stats.rx_bytes) FILTER (WHERE reports_latency), 0)::bigint AS workspace_rx_bytes,
	coalesce(SUM(stats.tx_bytes) FILTER (WHERE reports_latency), 0)::bigint AS workspace_tx_bytes,
	coalesce((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY stats.connection_median_latency_ms) FILTER (WHERE reports_latency)), -1)::FLOAT AS workspace_connection_latency_50,
	coalesce((PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY stats.connection_median_latency_ms) FILTER (WHERE reports_latency)), -1)::FLOAT AS workspace_connection_latency_95,
	-- Repeated so this row keeps the same layout as GetWorkspaceAgentStats, which
	-- telemetry converts between.
	stats.agent_id,
	coalesce(latest_sessions.session_counts, '{}'::jsonb)::jsonb AS session_counts
FROM stats
LEFT JOIN latest_sessions ON latest_sessions.agent_id = stats.agent_id
GROUP BY stats.user_id, stats.agent_id, stats.workspace_id, stats.template_id, latest_sessions.session_counts
HAVING BOOL_OR(reports_latency);

-- name: GetWorkspaceAgentStatsAndLabels :many
WITH agent_stats AS (
	SELECT
		user_id,
		agent_id,
		workspace_id,
		coalesce(SUM(rx_bytes), 0)::bigint AS rx_bytes,
		coalesce(SUM(tx_bytes), 0)::bigint AS tx_bytes
	 FROM workspace_agent_stats
		WHERE workspace_agent_stats.created_at > $1
		GROUP BY user_id, agent_id, workspace_id
), latest_stats AS (
	SELECT *, ROW_NUMBER() OVER(PARTITION BY agent_id ORDER BY created_at DESC) AS rn
	FROM workspace_agent_stats
	-- The greater than 0 is to support legacy agents that don't report connection_median_latency_ms.
	WHERE created_at > $1 AND connection_median_latency_ms > 0
), latest_sessions AS (
	-- Summed per app name here so the connection aggregates below keep seeing
	-- one row per agent instead of one row per app name.
	SELECT
		agent_id,
		coalesce(jsonb_object_agg(app_name, app_sessions), '{}'::jsonb)::jsonb AS session_counts
	FROM (
		SELECT
			a.agent_id,
			sess.app_name,
			SUM(sess.sessions::bigint) AS app_sessions
		FROM latest_stats AS a, jsonb_each_text(a.session_counts) AS sess(app_name, sessions)
		WHERE a.rn = 1
		GROUP BY a.agent_id, sess.app_name
	) AS app_totals
	GROUP BY agent_id
), latest_agent_stats AS (
	SELECT
		a.agent_id,
		coalesce(latest_sessions.session_counts, '{}'::jsonb)::jsonb AS session_counts,
		coalesce(SUM(a.connection_count), 0)::bigint AS connection_count,
		coalesce(MAX(a.connection_median_latency_ms), 0)::float AS connection_median_latency_ms
	FROM latest_stats AS a
	LEFT JOIN latest_sessions ON latest_sessions.agent_id = a.agent_id
	WHERE a.rn = 1
	GROUP BY a.user_id, a.agent_id, a.workspace_id, latest_sessions.session_counts
)
SELECT
	users.username, workspace_agents.name AS agent_name, workspaces.name AS workspace_name, rx_bytes, tx_bytes,
	session_counts,
	connection_count, connection_median_latency_ms
FROM
	agent_stats
JOIN
	latest_agent_stats
ON
	agent_stats.agent_id = latest_agent_stats.agent_id
JOIN
	users
ON
	users.id = agent_stats.user_id
JOIN
	workspace_agents
ON
	workspace_agents.id = agent_stats.agent_id
JOIN
	workspaces
ON
	workspaces.id = agent_stats.workspace_id;

-- name: GetWorkspaceAgentUsageStatsAndLabels :many
WITH agent_stats AS (
	SELECT
		user_id,
		agent_id,
		workspace_id,
		coalesce(SUM(rx_bytes), 0)::bigint AS rx_bytes,
		coalesce(SUM(tx_bytes), 0)::bigint AS tx_bytes,
		coalesce(MAX(connection_median_latency_ms), 0)::float AS connection_median_latency_ms
	FROM workspace_agent_stats
	-- The greater than 0 is to support legacy agents that don't report connection_median_latency_ms.
	WHERE workspace_agent_stats.created_at > $1 AND connection_median_latency_ms > 0
	GROUP BY user_id, agent_id, workspace_id
), latest_stats AS (
	SELECT *
	FROM workspace_agent_stats
	-- We only want the latest stats, but those stats might be
	-- spread across multiple rows.
	WHERE usage AND created_at > now() - '1 minute'::interval
), latest_sessions AS (
	-- Summed per app name here so the connection count below keeps seeing one
	-- row per agent instead of one row per app name.
	SELECT
		agent_id,
		coalesce(jsonb_object_agg(app_name, app_sessions), '{}'::jsonb)::jsonb AS session_counts
	FROM (
		SELECT
			latest_stats.agent_id,
			sess.app_name,
			SUM(sess.sessions::bigint) AS app_sessions
		FROM latest_stats, jsonb_each_text(latest_stats.session_counts) AS sess(app_name, sessions)
		GROUP BY latest_stats.agent_id, sess.app_name
	) AS app_totals
	GROUP BY agent_id
), latest_agent_stats AS (
	SELECT
		latest_stats.agent_id,
		coalesce(latest_sessions.session_counts, '{}'::jsonb)::jsonb AS session_counts,
		coalesce(SUM(latest_stats.connection_count), 0)::bigint AS connection_count
	FROM latest_stats
	LEFT JOIN latest_sessions ON latest_sessions.agent_id = latest_stats.agent_id
	GROUP BY latest_stats.user_id, latest_stats.agent_id, latest_stats.workspace_id, latest_sessions.session_counts
)
SELECT
	users.username, workspace_agents.name AS agent_name, workspaces.name AS workspace_name, rx_bytes, tx_bytes,
	coalesce(session_counts, '{}'::jsonb)::jsonb AS session_counts,
	coalesce(connection_count, 0)::bigint AS connection_count,
	connection_median_latency_ms
FROM
	agent_stats
LEFT JOIN
	latest_agent_stats
ON
	agent_stats.agent_id = latest_agent_stats.agent_id
JOIN
	users
ON
	users.id = agent_stats.user_id
JOIN
	workspace_agents
ON
	workspace_agents.id = agent_stats.agent_id
JOIN
	workspaces
ON
	workspaces.id = agent_stats.workspace_id;
