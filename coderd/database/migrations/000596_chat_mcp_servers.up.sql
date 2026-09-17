CREATE TABLE chat_mcp_servers (
    id UUID PRIMARY KEY,
    chat_id UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    slug TEXT NOT NULL,
    url TEXT NOT NULL,
    headers TEXT NOT NULL DEFAULT '{}',
    headers_key_id TEXT REFERENCES dbcrypt_keys(active_key_digest),
    tool_allow_list TEXT[] NOT NULL DEFAULT '{}',
    tool_deny_list TEXT[] NOT NULL DEFAULT '{}',
    allow_in_plan_mode BOOLEAN NOT NULL DEFAULT false,
    allow_in_subagents BOOLEAN NOT NULL DEFAULT false,
    forward_coder_headers BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (chat_id, slug)
);

COMMENT ON TABLE chat_mcp_servers IS 'MCP servers that a chat owner attached to a root chat. Experimental. chatd connects to every row on each turn of the chat.';
COMMENT ON COLUMN chat_mcp_servers.headers IS 'JSON object of HTTP header name to value sent on every request to the server. Encrypted at rest via dbcrypt when headers_key_id is set.';
COMMENT ON COLUMN chat_mcp_servers.headers_key_id IS 'The ID of the key used to encrypt headers. If this is NULL, headers are not encrypted.';
