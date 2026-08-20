CREATE TABLE chat_private_mcp_server_configs (
    chat_id UUID PRIMARY KEY REFERENCES chats (id) ON DELETE CASCADE,
    configs JSONB NOT NULL
);

COMMENT ON TABLE chat_private_mcp_server_configs IS 'POC-only chat-scoped private stateless MCP server configuration. Stored unencrypted and omitted from API responses and model prompts.';
