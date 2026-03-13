INSERT INTO chat_mcp_servers (
    id,
    slug,
    url,
    display_name,
    auth_type,
    enabled,
    created_by,
    created_at,
    updated_at
)
SELECT
    'a1b2c3d4-e5f6-7890-abcd-ef1234567890',
    'fixture-mcp-server',
    'https://example.com/mcp',
    'Fixture MCP Server',
    'none',
    TRUE,
    id,
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
FROM users
ORDER BY created_at, id
LIMIT 1;
