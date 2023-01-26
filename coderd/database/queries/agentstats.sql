-- name: InsertAgentStat :one
INSERT INTO
	agent_stats (
		id,
		created_at,
		user_id,
		workspace_id,
		template_id,
		agent_id,
		payload
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: GetTemplateDAUs :many
SELECT
	(created_at at TIME ZONE 'UTC')::date as date,
	user_id
FROM
	agent_stats
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
	agent_stats
GROUP BY
	date, user_id
ORDER BY
	date ASC;

-- name: DeleteOldAgentStats :exec
DELETE FROM agent_stats WHERE created_at < NOW() - INTERVAL '30 days';
