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

