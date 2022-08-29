-- name: InsertAgentStat :one
INSERT INTO
	agent_stats (
		id,
		created_at,
		user_id,
		workspace_id,
		agent_id,
		payload
	)
VALUES
	($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: GetDAUsFromAgentStats :many
select
	(created_at at TIME ZONE 'UTC')::date as date,
	count(distinct(user_id)) as daus
from
	agent_stats
group by
	date
order by
	date asc;

-- name: DeleteOldAgentStats :exec
DELETE FROM AGENT_STATS WHERE created_at  < now() - interval '30 days';
