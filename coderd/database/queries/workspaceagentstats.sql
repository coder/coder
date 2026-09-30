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
		connection_median_latency_ms
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
	unnest(@connection_median_latency_ms :: double precision[]) AS connection_median_latency_ms;

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
), latest_agent_stats AS (
	-- rn = 1 leaves one row per agent, and that row's session_counts is
	-- already the object this query reports, empty map included.
	SELECT
		agent_id,
		session_counts
	FROM latest_stats
	WHERE rn = 1
)
SELECT * FROM agent_stats JOIN latest_agent_stats ON agent_stats.agent_id = latest_agent_stats.agent_id;

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
), latest_agent_stats AS (
	-- rn = 1 leaves one row per agent, so the session object, connection
	-- count, and latency are that row's own columns, empty map included.
	SELECT
		agent_id,
		session_counts,
		connection_count,
		connection_median_latency_ms
	FROM latest_stats
	WHERE rn = 1
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
