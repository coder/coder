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
-- app_families maps each attributed family to its app names, so the probes
-- below stay one expression per family: adding a family needs a new list in
-- fams plus one probe here, because sqlc output columns are static.
-- fams turns the jsonb parameter into arrays once for the whole query. The
-- lateral decomposes session_counts once per row and sums each family from
-- that single pass, which measured ~3x faster than probing the jsonb once
-- per family. The subplan only runs for rows that survive the gate.
WITH fams AS (
	SELECT
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'vscode')) AS vscode,
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'ssh')) AS ssh,
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'jetbrains')) AS jetbrains,
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'reconnecting_pty')) AS reconnecting_pty
), stats AS (
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
	coalesce(SUM(sc.vscode) FILTER (WHERE rn = 1), 0)::bigint AS session_count_vscode,
	coalesce(SUM(sc.ssh) FILTER (WHERE rn = 1), 0)::bigint AS session_count_ssh,
	coalesce(SUM(sc.jetbrains) FILTER (WHERE rn = 1), 0)::bigint AS session_count_jetbrains,
	coalesce(SUM(sc.reconnecting_pty) FILTER (WHERE rn = 1), 0)::bigint AS session_count_reconnecting_pty
FROM stats
CROSS JOIN LATERAL (
	SELECT
		coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.vscode)), 0) AS vscode,
		coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.ssh)), 0) AS ssh,
		coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.jetbrains)), 0) AS jetbrains,
		coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.reconnecting_pty)), 0) AS reconnecting_pty
	FROM fams, jsonb_each_text(stats.session_counts) AS sess
	-- Gate on the same condition as the aggregate filters above so the
	-- decompose runs only for the rows that are counted (one-time filter).
	WHERE stats.rn = 1
) AS sc;

-- name: GetDeploymentWorkspaceAgentUsageStats :one
WITH fams AS (
	SELECT
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'vscode')) AS vscode,
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'ssh')) AS ssh,
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'jetbrains')) AS jetbrains,
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'reconnecting_pty')) AS reconnecting_pty
), agent_stats AS (
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
	SELECT
		coalesce(SUM(sc.vscode), 0)::bigint AS session_count_vscode,
		coalesce(SUM(sc.ssh), 0)::bigint AS session_count_ssh,
		coalesce(SUM(sc.jetbrains), 0)::bigint AS session_count_jetbrains,
		coalesce(SUM(sc.reconnecting_pty), 0)::bigint AS session_count_reconnecting_pty
	FROM
		latest_minutes
	JOIN
		workspace_agent_stats AS stats
	ON
		stats.agent_id = latest_minutes.agent_id
		AND stats.created_at >= $1
		AND stats.created_at >= latest_minutes.minute_bucket
		AND stats.created_at < latest_minutes.minute_bucket + '1 minute'::interval
		AND stats.usage
	CROSS JOIN LATERAL (
		SELECT
			coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.vscode)), 0) AS vscode,
			coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.ssh)), 0) AS ssh,
			coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.jetbrains)), 0) AS jetbrains,
			coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.reconnecting_pty)), 0) AS reconnecting_pty
		FROM fams, jsonb_each_text(stats.session_counts) AS sess
	) AS sc
)
SELECT * FROM agent_stats, latest_agent_stats;

-- name: GetWorkspaceAgentStats :many
WITH fams AS (
	SELECT
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'vscode')) AS vscode,
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'ssh')) AS ssh,
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'jetbrains')) AS jetbrains,
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'reconnecting_pty')) AS reconnecting_pty
), agent_stats AS (
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
		coalesce(SUM(sc.vscode), 0)::bigint AS session_count_vscode,
		coalesce(SUM(sc.ssh), 0)::bigint AS session_count_ssh,
		coalesce(SUM(sc.jetbrains), 0)::bigint AS session_count_jetbrains,
		coalesce(SUM(sc.reconnecting_pty), 0)::bigint AS session_count_reconnecting_pty
	 FROM (
		SELECT *, ROW_NUMBER() OVER(PARTITION BY agent_id ORDER BY created_at DESC) AS rn
		FROM workspace_agent_stats WHERE created_at > $1
	) AS a
	CROSS JOIN LATERAL (
		SELECT
			coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.vscode)), 0) AS vscode,
			coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.ssh)), 0) AS ssh,
			coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.jetbrains)), 0) AS jetbrains,
			coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.reconnecting_pty)), 0) AS reconnecting_pty
		FROM fams, jsonb_each_text(a.session_counts) AS sess
	) AS sc
	WHERE a.rn = 1
	GROUP BY a.user_id, a.agent_id, a.workspace_id, a.template_id
)
SELECT * FROM agent_stats JOIN latest_agent_stats ON agent_stats.agent_id = latest_agent_stats.agent_id;

-- name: GetWorkspaceAgentUsageStats :many
WITH fams AS (
	SELECT
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'vscode')) AS vscode,
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'ssh')) AS ssh,
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'jetbrains')) AS jetbrains,
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'reconnecting_pty')) AS reconnecting_pty
), stats AS (
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
)
SELECT
	user_id,
	agent_id,
	workspace_id,
	template_id,
	MIN(created_at) FILTER (WHERE reports_latency)::timestamptz AS aggregated_from,
	coalesce(SUM(rx_bytes) FILTER (WHERE reports_latency), 0)::bigint AS workspace_rx_bytes,
	coalesce(SUM(tx_bytes) FILTER (WHERE reports_latency), 0)::bigint AS workspace_tx_bytes,
	coalesce((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY connection_median_latency_ms) FILTER (WHERE reports_latency)), -1)::FLOAT AS workspace_connection_latency_50,
	coalesce((PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY connection_median_latency_ms) FILTER (WHERE reports_latency)), -1)::FLOAT AS workspace_connection_latency_95,
	-- Repeated so this row keeps the same layout as GetWorkspaceAgentStats, which
	-- telemetry converts between.
	agent_id,
	coalesce(SUM(sc.vscode) FILTER (WHERE in_latest_usage_minute), 0)::bigint AS session_count_vscode,
	coalesce(SUM(sc.ssh) FILTER (WHERE in_latest_usage_minute), 0)::bigint AS session_count_ssh,
	coalesce(SUM(sc.jetbrains) FILTER (WHERE in_latest_usage_minute), 0)::bigint AS session_count_jetbrains,
	coalesce(SUM(sc.reconnecting_pty) FILTER (WHERE in_latest_usage_minute), 0)::bigint AS session_count_reconnecting_pty
FROM stats
CROSS JOIN LATERAL (
	SELECT
		coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.vscode)), 0) AS vscode,
		coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.ssh)), 0) AS ssh,
		coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.jetbrains)), 0) AS jetbrains,
		coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.reconnecting_pty)), 0) AS reconnecting_pty
	FROM fams, jsonb_each_text(stats.session_counts) AS sess
	-- Gate on the same condition as the aggregate filters above so the
	-- decompose runs only for the rows that are counted (one-time filter).
	WHERE stats.in_latest_usage_minute
) AS sc
GROUP BY user_id, agent_id, workspace_id, template_id
HAVING BOOL_OR(reports_latency);

-- name: GetWorkspaceAgentStatsAndLabels :many
WITH fams AS (
	SELECT
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'vscode')) AS vscode,
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'ssh')) AS ssh,
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'jetbrains')) AS jetbrains,
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'reconnecting_pty')) AS reconnecting_pty
), agent_stats AS (
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
		WHERE created_at > $1 AND connection_median_latency_ms > 0
	) AS a
	CROSS JOIN LATERAL (
		SELECT
			coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.vscode)), 0) AS vscode,
			coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.ssh)), 0) AS ssh,
			coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.jetbrains)), 0) AS jetbrains,
			coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.reconnecting_pty)), 0) AS reconnecting_pty
		FROM fams, jsonb_each_text(a.session_counts) AS sess
	) AS sc
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
WITH fams AS (
	SELECT
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'vscode')) AS vscode,
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'ssh')) AS ssh,
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'jetbrains')) AS jetbrains,
		ARRAY(SELECT jsonb_array_elements_text(@app_families::jsonb -> 'reconnecting_pty')) AS reconnecting_pty
), agent_stats AS (
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
		agent_id,
		coalesce(SUM(sc.vscode), 0)::bigint AS session_count_vscode,
		coalesce(SUM(sc.ssh), 0)::bigint AS session_count_ssh,
		coalesce(SUM(sc.jetbrains), 0)::bigint AS session_count_jetbrains,
		coalesce(SUM(sc.reconnecting_pty), 0)::bigint AS session_count_reconnecting_pty,
		coalesce(SUM(connection_count), 0)::bigint AS connection_count
	FROM workspace_agent_stats
	CROSS JOIN LATERAL (
		SELECT
			coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.vscode)), 0) AS vscode,
			coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.ssh)), 0) AS ssh,
			coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.jetbrains)), 0) AS jetbrains,
			coalesce(SUM(sess.value::bigint) FILTER (WHERE sess.key = ANY (fams.reconnecting_pty)), 0) AS reconnecting_pty
		FROM fams, jsonb_each_text(workspace_agent_stats.session_counts) AS sess
	) AS sc
	-- We only want the latest stats, but those stats might be
	-- spread across multiple rows.
	WHERE usage AND created_at > now() - '1 minute'::interval
	GROUP BY user_id, agent_id, workspace_id
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
