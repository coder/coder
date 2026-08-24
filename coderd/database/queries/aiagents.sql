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

-- name: GetOrphanedChatAIAgents :many
-- Chat-origin AI agent identities whose chat no longer exists. Read before the
-- revocation below so that each one can be retired in the ledger through the
-- entity function, the ledger being a fold of its journal and not somewhere a
-- bulk statement may write directly.
SELECT user_id
FROM ai_agents
WHERE origin_type = 'chat'
	AND deleted = false
	AND NOT EXISTS (SELECT 1 FROM chats WHERE chats.id = ai_agents.origin_id);

-- name: RevokeOrphanedChatAIAgents :execrows
-- Marks chat-origin AI agent identities deleted when their chat no longer
-- exists (retention purge hard-deletes chats; ai_agents.origin_id has no
-- FK) and revokes their API keys. Idempotent.
WITH orphaned AS (
	UPDATE ai_agents
	SET deleted = true
	WHERE origin_type = 'chat'
		AND deleted = false
		AND NOT EXISTS (SELECT 1 FROM chats WHERE chats.id = ai_agents.origin_id)
	RETURNING user_id
)
DELETE FROM api_keys
WHERE holder_id IN (SELECT user_id FROM orphaned);
