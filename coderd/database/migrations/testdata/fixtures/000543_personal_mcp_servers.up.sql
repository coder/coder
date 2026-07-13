-- Personal MCP server config owned by the fixture admin user. It
-- reuses the slug of the global fixture row from 000447 to exercise
-- the partial unique indexes.
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
    owner_id,
    created_at,
    updated_at
) VALUES (
    'c3d4e5f6-a7b8-9012-cdef-123456789012',
    'Fixture Personal MCP Server',
    'fixture-mcp-server',
    'https://mcp.example.com/personal',
    'streamable_http',
    'none',
    'default_on',
    TRUE,
    '30095c71-380b-457a-8995-97b8ee6e5307', -- admin@coder.com
    '30095c71-380b-457a-8995-97b8ee6e5307', -- admin@coder.com
    '30095c71-380b-457a-8995-97b8ee6e5307', -- admin@coder.com
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
);
