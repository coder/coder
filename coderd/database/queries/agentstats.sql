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
select
	(created_at at TIME ZONE 'UTC')::date as date,
	user_id
from
	agent_stats
where template_id = $1
group by
	date, user_id
order by
	date asc;

-- name: DeleteOldAgentStats :exec
DELETE FROM AGENT_STATS WHERE created_at  < now() - interval '30 days';
