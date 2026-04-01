INSERT INTO mcp_server_configs (
    id,
    display_name,
    slug,
    url,
    transport,
    auth_type,
    availability,
    enabled,
    created_by,
    updated_by,
    created_at,
    updated_at
) VALUES (
    'a1b2c3d4-e5f6-7890-abcd-ef1234567890',
    'Fixture MCP Server',
    'fixture-mcp-server',
    'https://mcp.example.com/sse',
    'sse',
    'none',
    'default_on',
    TRUE,
    '30095c71-380b-457a-8995-97b8ee6e5307', -- admin@coder.com
    '30095c71-380b-457a-8995-97b8ee6e5307', -- admin@coder.com
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
);

INSERT INTO mcp_server_user_tokens (
    id,
    mcp_server_config_id,
    user_id,
    access_token,
    token_type,
    created_at,
    updated_at
)
SELECT
    'b2c3d4e5-f6a7-8901-bcde-f12345678901',
    'a1b2c3d4-e5f6-7890-abcd-ef1234567890',
    id,
    'fixture-access-token',
    'Bearer',
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
FROM users
ORDER BY created_at, id
LIMIT 1;
