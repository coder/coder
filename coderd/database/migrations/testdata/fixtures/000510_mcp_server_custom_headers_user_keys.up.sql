-- Migration 500 adds custom_headers_user_keys to mcp_server_configs and
-- creates mcp_server_user_header_values. Insert a fixture row exercising
-- the user-set header values flow.

INSERT INTO mcp_server_configs (
    id,
    display_name,
    slug,
    url,
    transport,
    auth_type,
    custom_headers,
    custom_headers_user_keys,
    availability,
    enabled,
    created_by,
    updated_by,
    created_at,
    updated_at
) VALUES (
    'c3d4e5f6-a7b8-9012-cdef-123456789012',
    'Fixture User-Set Headers MCP Server',
    'fixture-user-set-headers-mcp-server',
    'https://mcp.example.com/streamable',
    'streamable_http',
    'custom_headers',
    '{"X-Org-ID":"acme"}',
    ARRAY['X-User-Token'],
    'default_off',
    TRUE,
    '30095c71-380b-457a-8995-97b8ee6e5307', -- admin@coder.com
    '30095c71-380b-457a-8995-97b8ee6e5307', -- admin@coder.com
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
);

INSERT INTO mcp_server_user_header_values (
    id,
    mcp_server_config_id,
    user_id,
    header_values,
    created_at,
    updated_at
) VALUES (
    'd4e5f6a7-b8c9-0123-defa-234567890123',
    'c3d4e5f6-a7b8-9012-cdef-123456789012',
    '30095c71-380b-457a-8995-97b8ee6e5307', -- admin@coder.com
    '{"X-User-Token":"user-supplied-token"}',
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
);
