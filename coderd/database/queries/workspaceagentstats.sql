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

-- name: GetTemplateDAUs :many
SELECT
	(created_at at TIME ZONE 'UTC')::date as date,
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
	(created_at at TIME ZONE 'UTC')::date as date,
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
DELETE FROM workspace_agent_stats WHERE created_at < NOW() - INTERVAL '30 days';

-- name: GetDeploymentWorkspaceAgentStats :one
WITH agent_stats AS (
	SELECT
		coalesce(SUM(rx_bytes), 0)::bigint AS workspace_rx_bytes,
		coalesce(SUM(tx_bytes), 0)::bigint AS workspace_tx_bytes,
		coalesce((PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY connection_median_latency_ms)), -1)::FLOAT AS workspace_connection_latency_50,
		coalesce((PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY connection_median_latency_ms)), -1)::FLOAT AS workspace_connection_latency_95
	 FROM workspace_agent_stats
		WHERE workspace_agent_stats.created_at > $1
), latest_agent_stats AS (
	SELECT
		coalesce(SUM(session_count_vscode), 0)::bigint AS session_count_vscode,
		coalesce(SUM(session_count_ssh), 0)::bigint AS session_count_ssh,
		coalesce(SUM(session_count_jetbrains), 0)::bigint AS session_count_jetbrains,
		coalesce(SUM(session_count_reconnecting_pty), 0)::bigint AS session_count_reconnecting_pty
	 FROM (
		SELECT *, ROW_NUMBER() OVER(PARTITION BY agent_id ORDER BY created_at DESC) AS rn
		FROM workspace_agent_stats
	) AS a WHERE a.rn = 1
)
SELECT * FROM agent_stats, latest_agent_stats;
