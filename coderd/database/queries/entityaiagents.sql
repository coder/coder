-- name: InsertEntityAIAgent :one
INSERT INTO
	entity_ai_agents (id, owner_id)
VALUES
	($1, $2) RETURNING *;

-- name: GetEntityAIAgentByID :one
SELECT
	*
FROM
	entity_ai_agents
WHERE
	id = $1;
