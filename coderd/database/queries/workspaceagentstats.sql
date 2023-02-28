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
		tx_bytes
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING *;

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
