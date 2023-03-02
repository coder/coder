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
	template_id = $1
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
GROUP BY
	date, user_id
ORDER BY
	date ASC;

-- name: DeleteOldWorkspaceAgentStats :exec
DELETE FROM workspace_agent_stats WHERE created_at < NOW() - INTERVAL '30 days';

-- name: GetDeploymentWorkspaceAgentStats :one
WITH agent_stats AS (
	SELECT * FROM workspace_agent_stats
		WHERE created_at > $1
), latest_agent_stats AS (
	SELECT * FROM agent_stats GROUP BY agent_id ORDER BY created_at
)
SELECT
	SUM(latest_agent_stats.session_count_vscode) AS session_count_vscode,
	SUM(latest_agent_stats.session_count_ssh) AS session_count_ssh,
	SUM(latest_agent_stats.session_count_jetbrains) AS session_count_jetbrains,
	SUM(latest_agent_stats.session_count_reconnecting_pty) AS session_count_reconnecting_pty,
	SUM(agent_stats.rx_bytes) AS workspace_rx_bytes,
	SUM(agent_stats.tx_bytes) AS workspace_tx_bytes,
	coalesce((PERCENTILE_DISC(0.5) WITHIN GROUP(ORDER BY agent_stats.connection_median_latency_ms)), -1)::FLOAT AS workspace_connection_latency_50,
	coalesce((PERCENTILE_DISC(0.95) WITHIN GROUP(ORDER BY agent_stats.connection_median_latency_ms)), -1)::FLOAT AS workspace_connection_latency_95
 FROM agent_stats JOIN latest_agent_stats ON agent_stats.agent_id = latest_agent_stats.agent_id;
