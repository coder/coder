-- name: GetChatMCPServerByID :one
SELECT * FROM chat_mcp_servers WHERE id = @id::uuid;

-- name: GetChatMCPServers :many
SELECT * FROM chat_mcp_servers ORDER BY slug ASC;

-- name: GetEnabledChatMCPServers :many
SELECT * FROM chat_mcp_servers WHERE enabled = TRUE ORDER BY slug ASC;

-- name: InsertChatMCPServer :one
INSERT INTO chat_mcp_servers (
    slug, url, display_name, auth_type, auth_headers, auth_headers_key_id,
    oauth_client_id, oauth_auth_server, tool_allow_regex, tool_deny_regex,
    enabled, created_by
) VALUES (
    @slug::text, @url::text, @display_name::text, @auth_type::text,
    @auth_headers::text, sqlc.narg('auth_headers_key_id')::text,
    @oauth_client_id::text, @oauth_auth_server::text,
    @tool_allow_regex::text, @tool_deny_regex::text,
    @enabled::boolean, sqlc.narg('created_by')::uuid
) RETURNING *;

-- name: UpdateChatMCPServer :one
UPDATE chat_mcp_servers SET
    slug = @slug::text,
    url = @url::text,
    display_name = @display_name::text,
    auth_type = @auth_type::text,
    auth_headers = @auth_headers::text,
    auth_headers_key_id = sqlc.narg('auth_headers_key_id')::text,
    oauth_client_id = @oauth_client_id::text,
    oauth_auth_server = @oauth_auth_server::text,
    tool_allow_regex = @tool_allow_regex::text,
    tool_deny_regex = @tool_deny_regex::text,
    enabled = @enabled::boolean,
    updated_at = NOW()
WHERE id = @id::uuid
RETURNING *;

-- name: DeleteChatMCPServerByID :exec
DELETE FROM chat_mcp_servers WHERE id = @id::uuid;
