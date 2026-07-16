-- name: InsertWorkspaceAgentStats :exec
WITH stats AS (
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
		unnest(@connection_median_latency_ms :: double precision[]) AS connection_median_latency_ms,
		unnest(@usage :: boolean[]) AS usage
),
session_data AS (
	-- @session_counts is a JSONB array with one object per stats row (zipped
	-- with @id by position, same contract as @connections_by_proto), each
	-- object mapping app name to session count.
	SELECT
		unnest(@id :: uuid[]) AS stats_id,
		unnest(@created_at :: timestamptz[]) AS created_at,
		jsonb_array_elements(@session_counts :: jsonb) AS counts
)
INSERT INTO workspace_agent_session_counts (workspace_agent_stats_id, created_at, app_name, count)
SELECT
	sd.stats_id,
	sd.created_at,
	kv.key,
	(kv.value)::bigint
FROM session_data sd, jsonb_each_text(sd.counts) kv
WHERE (kv.value)::bigint > 0;

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
WITH stats AS (
    SELECT
        id,
        agent_id,
        created_at,
        rx_bytes,
        tx_bytes,
        connection_median_latency_ms,
        ROW_NUMBER() OVER (PARTITION BY agent_id ORDER BY created_at DESC) AS rn
    FROM workspace_agent_stats
    WHERE workspace_agent_stats.created_at > $1
), byte_stats AS (
    SELECT
        coalesce(SUM(rx_bytes), 0)::bigint AS workspace_rx_bytes,
        coalesce(SUM(tx_bytes), 0)::bigint AS workspace_tx_bytes,
        -- The greater than 0 is to support legacy agents that don't report connection_median_latency_ms.
        coalesce((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY connection_median_latency_ms) FILTER (WHERE connection_median_latency_ms > 0)), -1)::FLOAT AS workspace_connection_latency_50,
        coalesce((PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY connection_median_latency_ms) FILTER (WHERE connection_median_latency_ms > 0)), -1)::FLOAT AS workspace_connection_latency_95
    FROM stats
), session_stats AS (
    -- Session counts from the latest stats row per agent.
    SELECT
        coalesce(SUM(sc.count) FILTER (WHERE sc.app_name = 'vscode'), 0)::bigint AS session_count_vscode,
        coalesce(SUM(sc.count) FILTER (WHERE sc.app_name = 'ssh'), 0)::bigint AS session_count_ssh,
        coalesce(SUM(sc.count) FILTER (WHERE sc.app_name = 'jetbrains'), 0)::bigint AS session_count_jetbrains,
        coalesce(SUM(sc.count) FILTER (WHERE sc.app_name = 'reconnecting_pty'), 0)::bigint AS session_count_reconnecting_pty
    FROM stats
    LEFT JOIN workspace_agent_session_counts sc ON sc.workspace_agent_stats_id = stats.id AND sc.created_at > $1
    WHERE stats.rn = 1
)
SELECT * FROM byte_stats, session_stats;

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
minute_buckets AS (
	SELECT
		was.agent_id,
		date_trunc('minute', was.created_at) AS minute_bucket,
		coalesce(SUM(sc.count) FILTER (WHERE sc.app_name = 'vscode'), 0)::bigint AS session_count_vscode,
		coalesce(SUM(sc.count) FILTER (WHERE sc.app_name = 'ssh'), 0)::bigint AS session_count_ssh,
		coalesce(SUM(sc.count) FILTER (WHERE sc.app_name = 'jetbrains'), 0)::bigint AS session_count_jetbrains,
		coalesce(SUM(sc.count) FILTER (WHERE sc.app_name = 'reconnecting_pty'), 0)::bigint AS session_count_reconnecting_pty
	FROM
		workspace_agent_stats was
	LEFT JOIN workspace_agent_session_counts sc ON sc.workspace_agent_stats_id = was.id AND sc.created_at >= $1
	WHERE
		was.created_at >= $1
		AND was.created_at < date_trunc('minute', now())  -- Exclude current partial minute
		AND was.usage = true
	GROUP BY
		was.agent_id,
		minute_bucket
),
latest_buckets AS (
	SELECT DISTINCT ON (agent_id)
		agent_id,
		minute_bucket,
		session_count_vscode,
		session_count_jetbrains,
		session_count_reconnecting_pty,
		session_count_ssh
	FROM
		minute_buckets
	ORDER BY
		agent_id,
		minute_bucket DESC
),
latest_agent_stats AS (
    SELECT
		coalesce(SUM(session_count_vscode), 0)::bigint AS session_count_vscode,
		coalesce(SUM(session_count_ssh), 0)::bigint AS session_count_ssh,
		coalesce(SUM(session_count_jetbrains), 0)::bigint AS session_count_jetbrains,
		coalesce(SUM(session_count_reconnecting_pty), 0)::bigint AS session_count_reconnecting_pty
    FROM
        latest_buckets
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
), latest_agent_stats AS (
	SELECT
		a.agent_id,
		coalesce(SUM(sc.count) FILTER (WHERE sc.app_name = 'vscode'), 0)::bigint AS session_count_vscode,
		coalesce(SUM(sc.count) FILTER (WHERE sc.app_name = 'ssh'), 0)::bigint AS session_count_ssh,
		coalesce(SUM(sc.count) FILTER (WHERE sc.app_name = 'jetbrains'), 0)::bigint AS session_count_jetbrains,
		coalesce(SUM(sc.count) FILTER (WHERE sc.app_name = 'reconnecting_pty'), 0)::bigint AS session_count_reconnecting_pty
	 FROM (
		SELECT *, ROW_NUMBER() OVER(PARTITION BY agent_id ORDER BY created_at DESC) AS rn
		FROM workspace_agent_stats WHERE workspace_agent_stats.created_at > $1
	) AS a
	LEFT JOIN workspace_agent_session_counts sc ON sc.workspace_agent_stats_id = a.id AND sc.created_at > $1
	WHERE a.rn = 1 GROUP BY a.user_id, a.agent_id, a.workspace_id, a.template_id
)
SELECT * FROM agent_stats JOIN latest_agent_stats ON agent_stats.agent_id = latest_agent_stats.agent_id;

-- name: GetWorkspaceAgentUsageStats :many
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
),
minute_buckets AS (
	SELECT
		was.agent_id,
		date_trunc('minute', was.created_at) AS minute_bucket,
		coalesce(SUM(sc.count) FILTER (WHERE sc.app_name = 'vscode'), 0)::bigint AS session_count_vscode,
		coalesce(SUM(sc.count) FILTER (WHERE sc.app_name = 'ssh'), 0)::bigint AS session_count_ssh,
		coalesce(SUM(sc.count) FILTER (WHERE sc.app_name = 'jetbrains'), 0)::bigint AS session_count_jetbrains,
		coalesce(SUM(sc.count) FILTER (WHERE sc.app_name = 'reconnecting_pty'), 0)::bigint AS session_count_reconnecting_pty
	FROM
		workspace_agent_stats was
	LEFT JOIN workspace_agent_session_counts sc ON sc.workspace_agent_stats_id = was.id AND sc.created_at >= $1
	WHERE
		was.created_at >= $1
		AND was.created_at < date_trunc('minute', now())  -- Exclude current partial minute
		AND was.usage = true
	GROUP BY
		was.agent_id,
		minute_bucket,
		was.user_id,
		was.agent_id,
		was.workspace_id,
		was.template_id
),
latest_buckets AS (
	SELECT DISTINCT ON (agent_id)
		agent_id,
		session_count_vscode,
		session_count_ssh,
		session_count_jetbrains,
		session_count_reconnecting_pty
	FROM
		minute_buckets
	ORDER BY
		agent_id,
		minute_bucket DESC
)
SELECT user_id,
agent_stats.agent_id,
workspace_id,
template_id,
aggregated_from,
workspace_rx_bytes,
workspace_tx_bytes,
workspace_connection_latency_50,
workspace_connection_latency_95,
-- `minute_buckets` could return 0 rows if there are no usage stats since `created_at`.
coalesce(latest_buckets.agent_id,agent_stats.agent_id) AS agent_id,
coalesce(session_count_vscode, 0)::bigint AS session_count_vscode,
coalesce(session_count_ssh, 0)::bigint AS session_count_ssh,
coalesce(session_count_jetbrains, 0)::bigint AS session_count_jetbrains,
coalesce(session_count_reconnecting_pty, 0)::bigint AS session_count_reconnecting_pty
FROM agent_stats LEFT JOIN latest_buckets ON agent_stats.agent_id = latest_buckets.agent_id;

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
), latest_agent_stats AS (
	SELECT
		a.agent_id,
		coalesce(SUM(sc.vscode), 0)::bigint AS session_count_vscode,
		coalesce(SUM(sc.ssh), 0)::bigint AS session_count_ssh,
		coalesce(SUM(sc.jetbrains), 0)::bigint AS session_count_jetbrains,
		coalesce(SUM(sc.reconnecting_pty), 0)::bigint AS session_count_reconnecting_pty,
		coalesce(SUM(a.connection_count), 0)::bigint AS connection_count,
		coalesce(MAX(a.connection_median_latency_ms), 0)::float AS connection_median_latency_ms
	 FROM (
		SELECT *, ROW_NUMBER() OVER(PARTITION BY agent_id ORDER BY created_at DESC) AS rn
		FROM workspace_agent_stats
		-- The greater than 0 is to support legacy agents that don't report connection_median_latency_ms.
		WHERE workspace_agent_stats.created_at > $1 AND connection_median_latency_ms > 0
	) AS a
	-- Pre-aggregate to one row per stats row; a direct join would fan out
	-- the SUM(connection_count) above.
	LEFT JOIN LATERAL (
		SELECT
			coalesce(SUM(count) FILTER (WHERE app_name = 'vscode'), 0)::bigint AS vscode,
			coalesce(SUM(count) FILTER (WHERE app_name = 'ssh'), 0)::bigint AS ssh,
			coalesce(SUM(count) FILTER (WHERE app_name = 'jetbrains'), 0)::bigint AS jetbrains,
			coalesce(SUM(count) FILTER (WHERE app_name = 'reconnecting_pty'), 0)::bigint AS reconnecting_pty
		FROM workspace_agent_session_counts
		WHERE workspace_agent_stats_id = a.id
	) sc ON TRUE
	WHERE a.rn = 1
	GROUP BY a.user_id, a.agent_id, a.workspace_id
)
SELECT
	users.username, workspace_agents.name AS agent_name, workspaces.name AS workspace_name, rx_bytes, tx_bytes,
	session_count_vscode, session_count_ssh, session_count_jetbrains, session_count_reconnecting_pty,
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
), latest_agent_stats AS (
	SELECT
		was.agent_id,
		coalesce(SUM(sc.vscode), 0)::bigint AS session_count_vscode,
		coalesce(SUM(sc.ssh), 0)::bigint AS session_count_ssh,
		coalesce(SUM(sc.jetbrains), 0)::bigint AS session_count_jetbrains,
		coalesce(SUM(sc.reconnecting_pty), 0)::bigint AS session_count_reconnecting_pty,
		coalesce(SUM(was.connection_count), 0)::bigint AS connection_count
	FROM workspace_agent_stats was
	-- Pre-aggregate to one row per stats row; a direct join would fan out
	-- the SUM(connection_count) above.
	LEFT JOIN LATERAL (
		SELECT
			coalesce(SUM(count) FILTER (WHERE app_name = 'vscode'), 0)::bigint AS vscode,
			coalesce(SUM(count) FILTER (WHERE app_name = 'ssh'), 0)::bigint AS ssh,
			coalesce(SUM(count) FILTER (WHERE app_name = 'jetbrains'), 0)::bigint AS jetbrains,
			coalesce(SUM(count) FILTER (WHERE app_name = 'reconnecting_pty'), 0)::bigint AS reconnecting_pty
		FROM workspace_agent_session_counts
		WHERE workspace_agent_stats_id = was.id
	) sc ON TRUE
	-- We only want the latest stats, but those stats might be
	-- spread across multiple rows.
	WHERE was.usage = true AND was.created_at > now() - '1 minute'::interval
	GROUP BY was.user_id, was.agent_id, was.workspace_id
)
SELECT
	users.username, workspace_agents.name AS agent_name, workspaces.name AS workspace_name, rx_bytes, tx_bytes,
	coalesce(session_count_vscode, 0)::bigint AS session_count_vscode,
	coalesce(session_count_ssh, 0)::bigint AS session_count_ssh,
	coalesce(session_count_jetbrains, 0)::bigint AS session_count_jetbrains,
	coalesce(session_count_reconnecting_pty, 0)::bigint AS session_count_reconnecting_pty,
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
