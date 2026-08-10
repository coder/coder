-- name: InsertAIAgentUser :one
INSERT INTO users (
	id,
	email,
	username,
	name,
	hashed_password,
	created_at,
	updated_at,
	rbac_roles,
	login_type,
	status,
	is_service_account,
	kind
) VALUES (
	@id,
	'',
	@username,
	'',
	''::bytea,
	@created_at,
	@created_at,
	'{}'::text[],
	'none'::login_type,
	'active'::user_status,
	false,
	'ai_agent'::user_kind
)
RETURNING *;

-- name: InsertAIAgent :one
INSERT INTO ai_agents (
	user_id,
	owner_user_id,
	origin_type,
	origin_id,
	created_at,
	deleted
) VALUES (
	@user_id,
	@owner_user_id,
	@origin_type,
	@origin_id,
	@created_at,
	false
)
RETURNING *;

-- name: GetAIAgentByUserID :one
SELECT *
FROM ai_agents
WHERE user_id = @user_id;

-- name: GetAIAgentByOrigin :one
SELECT *
FROM ai_agents
WHERE origin_type = @origin_type
	AND origin_id = @origin_id
	AND deleted = false;

-- name: GetAIAgentsByOwnerID :many
SELECT
	sqlc.embed(ai_agents),
	users.username
FROM ai_agents
INNER JOIN users ON users.id = ai_agents.user_id
WHERE ai_agents.owner_user_id = @owner_user_id
ORDER BY ai_agents.created_at DESC;

-- name: UpdateAIAgentDeleted :one
UPDATE ai_agents
SET deleted = @deleted
WHERE user_id = @user_id
RETURNING *;
