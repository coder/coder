ALTER TABLE mcp_server_configs
    DROP CONSTRAINT mcp_server_configs_auth_type_check;

ALTER TABLE mcp_server_configs
    ADD CONSTRAINT mcp_server_configs_auth_type_check
    CHECK (auth_type IN ('none', 'oauth2', 'api_key', 'custom_headers', 'user_oidc'));
