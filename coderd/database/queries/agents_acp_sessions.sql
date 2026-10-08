-- name: InsertAgentsACPSession :one
INSERT INTO agents_acp_sessions (
    id, organization_id, chat_id, workspace_id,
    working_directory, harness_slug, harness_display_name, session_id
)
SELECT
    @id::uuid, chats.organization_id, chats.id, chats.workspace_id,
    @working_directory::text, @harness_slug::text, @harness_display_name::text, @session_id::text
FROM chats
WHERE chats.id = @chat_id::uuid
    AND chats.organization_id = @organization_id::uuid
    AND chats.workspace_id = @workspace_id::uuid
    AND chats.agent_id = @agent_id::uuid
ON CONFLICT (id) DO UPDATE SET id = agents_acp_sessions.id
WHERE agents_acp_sessions.chat_id = EXCLUDED.chat_id
    AND agents_acp_sessions.workspace_id = EXCLUDED.workspace_id
    AND agents_acp_sessions.harness_slug = EXCLUDED.harness_slug
    AND agents_acp_sessions.working_directory = EXCLUDED.working_directory
    AND agents_acp_sessions.session_id = EXCLUDED.session_id
RETURNING *;

-- name: GetAgentsACPSessionByIDAndChatID :one
SELECT * FROM agents_acp_sessions
WHERE id = @id::uuid AND chat_id = @chat_id::uuid;

-- name: ListAgentsACPSessionsByChatID :many
SELECT * FROM agents_acp_sessions
WHERE chat_id = @chat_id::uuid
ORDER BY updated_at DESC, id
LIMIT @limit_value::int OFFSET @offset_value::int;

-- name: CountAgentsACPSessionsByChatID :one
SELECT count(*) FROM agents_acp_sessions WHERE chat_id = @chat_id::uuid;

-- name: UpdateAgentsACPSessionUpdatedAt :one
UPDATE agents_acp_sessions AS sessions
SET updated_at = now()
FROM chats
WHERE sessions.id = @id::uuid AND sessions.chat_id = @chat_id::uuid
    AND chats.id = sessions.chat_id
    AND chats.organization_id = sessions.organization_id
    AND chats.workspace_id = sessions.workspace_id
    AND chats.agent_id = @agent_id::uuid
RETURNING sessions.id;
