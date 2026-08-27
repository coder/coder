ALTER TABLE mcp_server_configs
    DROP CONSTRAINT mcp_server_configs_auth_type_check;

ALTER TABLE mcp_server_configs
    ADD CONSTRAINT mcp_server_configs_auth_type_check
    CHECK (auth_type IN ('none', 'oauth2', 'api_key', 'custom_headers', 'user_oidc', 'external_auth'));

ALTER TABLE mcp_server_configs
    ADD COLUMN external_auth_provider_id text,
    ADD COLUMN tool_rules jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN tool_default text NOT NULL DEFAULT 'enabled',
    ADD CONSTRAINT mcp_server_configs_tool_rules_check
        CHECK (jsonb_typeof(tool_rules) = 'array'),
    ADD CONSTRAINT mcp_server_configs_tool_default_check
        CHECK (tool_default IN ('enabled', 'disabled'));

ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'mcp_server_config';
