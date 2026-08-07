INSERT INTO mcp_server_configs (
    id,
    organization_id,
    display_name,
    slug,
    url,
    auth_type,
    group_acl
) VALUES (
    'f5670000-0000-4000-8000-000000000001',
    'f5610000-0000-4000-8000-000000000001',
    'Fixture MCP ACL Server',
    'fixture-mcp-acl-server',
    'https://mcp.example.com/acl',
    'none',
    '{"f5610000-0000-4000-8000-000000000001":{"permissions":["read"]}}'::JSONB
);
