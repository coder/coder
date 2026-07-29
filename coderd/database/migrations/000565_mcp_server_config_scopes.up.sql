ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:delete';
