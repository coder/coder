-- The earlier chat fixtures already insert a chat. Attach one private MCP
-- configuration so migration tests exercise the server-only table.
INSERT INTO chat_private_mcp_server_configs (chat_id, configs)
SELECT
    id,
    '[{"name":"fixture","url":"https://example.com/mcp"}]'::jsonb
FROM chats
ORDER BY created_at, id
LIMIT 1;
