-- name: InsertAIAgent :one
INSERT INTO
	ai_agents (id, owner_id)
VALUES
	($1, $2) RETURNING *;

-- name: GetAIAgentByID :one
SELECT
	*
FROM
	ai_agents
WHERE
	id = $1;
