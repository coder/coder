-- name: GetChatMCPServersByChatID :many
SELECT
    *
FROM
    chat_mcp_servers
WHERE
    chat_id = @chat_id::uuid
ORDER BY
    slug ASC;

-- name: UpsertChatMCPServer :one
INSERT INTO chat_mcp_servers (
    id,
    chat_id,
    slug,
    url,
    headers,
    headers_key_id,
    tool_allow_list,
    tool_deny_list,
    allow_in_subagents,
    forward_coder_headers
) VALUES (
    @id::uuid,
    @chat_id::uuid,
    @slug::text,
    @url::text,
    @headers::text,
    sqlc.narg('headers_key_id')::text,
    @tool_allow_list::text[],
    @tool_deny_list::text[],
    @allow_in_subagents::boolean,
    @forward_coder_headers::boolean
)
ON CONFLICT (chat_id, slug) DO UPDATE SET
    url = EXCLUDED.url,
    headers = EXCLUDED.headers,
    headers_key_id = EXCLUDED.headers_key_id,
    tool_allow_list = EXCLUDED.tool_allow_list,
    tool_deny_list = EXCLUDED.tool_deny_list,
    allow_in_subagents = EXCLUDED.allow_in_subagents,
    forward_coder_headers = EXCLUDED.forward_coder_headers,
    updated_at = now()
RETURNING
    *;

-- name: DeleteChatMCPServersByChatIDExcludingSlugs :exec
DELETE FROM
    chat_mcp_servers
WHERE
    chat_id = @chat_id::uuid
    AND NOT (slug = ANY(@slugs::text[]));

-- name: GetChatMCPServersByChatOwnerID :many
SELECT
    cms.*
FROM
    chat_mcp_servers cms
JOIN
    chats ON chats.id = cms.chat_id
WHERE
    chats.owner_id = @owner_id::uuid
ORDER BY
    cms.id ASC;

-- name: UpdateEncryptedChatMCPServerHeaders :exec
UPDATE
    chat_mcp_servers
SET
    headers = @headers::text,
    headers_key_id = sqlc.narg('headers_key_id')::text
WHERE
    id = @id::uuid;
