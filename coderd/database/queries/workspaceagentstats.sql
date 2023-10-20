-- name: InsertWorkspaceAgentStat :one
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
		session_count_vscode,
		session_count_jetbrains,
		session_count_reconnecting_pty,
		session_count_ssh,
		connection_median_latency_ms
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17) RETURNING *;

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
		session_count_vscode,
		session_count_jetbrains,
		session_count_reconnecting_pty,
		session_count_ssh,
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
	unnest(@session_count_vscode :: bigint[]) AS session_count_vscode,
	unnest(@session_count_jetbrains :: bigint[]) AS session_count_jetbrains,
	unnest(@session_count_reconnecting_pty :: bigint[]) AS session_count_reconnecting_pty,
	unnest(@session_count_ssh :: bigint[]) AS session_count_ssh,
	unnest(@connection_median_latency_ms :: double precision[]) AS connection_median_latency_ms;

-- name: GetTemplateDAUs :many
SELECT
	(created_at at TIME ZONE cast(@tz_offset::integer as text))::date as date,
	user_id
FROM
	workspace_agent_stats
WHERE
	template_id = $1 AND
	connection_count > 0
GROUP BY
	date, user_id
ORDER BY
	date ASC;

-- name: GetDeploymentDAUs :many
SELECT
	(created_at at TIME ZONE cast(@tz_offset::integer as text))::date as date,
	user_id
FROM
	workspace_agent_stats
WHERE
	connection_count > 0
GROUP BY
	date, user_id
ORDER BY
	date ASC;

-- name: DeleteOldWorkspaceAgentStats :exec
DELETE FROM workspace_agent_stats WHERE created_at < NOW() - INTERVAL '6 months';

-- name: GetDeploymentWorkspaceAgentStats :one
WITH agent_stats AS (
	SELECT
		coalesce(SUM(rx_bytes), 0)::bigint AS workspace_rx_bytes,
		coalesce(SUM(tx_bytes), 0)::bigint AS workspace_tx_bytes,
		coalesce((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY connection_median_latency_ms)), -1)::FLOAT AS workspace_connection_latency_50,
		coalesce((PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY connection_median_latency_ms)), -1)::FLOAT AS workspace_connection_latency_95
	 FROM workspace_agent_stats
	 	-- The greater than 0 is to support legacy agents that don't report connection_median_latency_ms.
		WHERE workspace_agent_stats.created_at > $1 AND connection_median_latency_ms > 0
), latest_agent_stats AS (
	SELECT
		coalesce(SUM(session_count_vscode), 0)::bigint AS session_count_vscode,
		coalesce(SUM(session_count_ssh), 0)::bigint AS session_count_ssh,
		coalesce(SUM(session_count_jetbrains), 0)::bigint AS session_count_jetbrains,
		coalesce(SUM(session_count_reconnecting_pty), 0)::bigint AS session_count_reconnecting_pty
	 FROM (
		SELECT *, ROW_NUMBER() OVER(PARTITION BY agent_id ORDER BY created_at DESC) AS rn
		FROM workspace_agent_stats WHERE created_at > $1
	) AS a WHERE a.rn = 1
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
		WHERE workspace_agent_stats.created_at > $1 AND connection_median_latency_ms > 0 GROUP BY user_id, agent_id, workspace_id, template_id
), latest_agent_stats AS (
	SELECT
		a.agent_id,
		coalesce(SUM(session_count_vscode), 0)::bigint AS session_count_vscode,
		coalesce(SUM(session_count_ssh), 0)::bigint AS session_count_ssh,
		coalesce(SUM(session_count_jetbrains), 0)::bigint AS session_count_jetbrains,
		coalesce(SUM(session_count_reconnecting_pty), 0)::bigint AS session_count_reconnecting_pty
	 FROM (
		SELECT *, ROW_NUMBER() OVER(PARTITION BY agent_id ORDER BY created_at DESC) AS rn
		FROM workspace_agent_stats WHERE created_at > $1
	) AS a WHERE a.rn = 1 GROUP BY a.user_id, a.agent_id, a.workspace_id, a.template_id
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
), latest_agent_stats AS (
	SELECT
		a.agent_id,
		coalesce(SUM(session_count_vscode), 0)::bigint AS session_count_vscode,
		coalesce(SUM(session_count_ssh), 0)::bigint AS session_count_ssh,
		coalesce(SUM(session_count_jetbrains), 0)::bigint AS session_count_jetbrains,
		coalesce(SUM(session_count_reconnecting_pty), 0)::bigint AS session_count_reconnecting_pty,
		coalesce(SUM(connection_count), 0)::bigint AS connection_count,
		coalesce(MAX(connection_median_latency_ms), 0)::float AS connection_median_latency_ms
	 FROM (
		SELECT *, ROW_NUMBER() OVER(PARTITION BY agent_id ORDER BY created_at DESC) AS rn
		FROM workspace_agent_stats
		-- The greater than 0 is to support legacy agents that don't report connection_median_latency_ms.
		WHERE created_at > $1 AND connection_median_latency_ms > 0
	) AS a
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
