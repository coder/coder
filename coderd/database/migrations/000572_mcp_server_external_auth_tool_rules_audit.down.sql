-- Rows using external_auth cannot satisfy the restored auth type constraint.
DELETE FROM mcp_server_configs WHERE auth_type = 'external_auth';

ALTER TABLE mcp_server_configs
    DROP CONSTRAINT mcp_server_configs_auth_type_check;

ALTER TABLE mcp_server_configs
    ADD CONSTRAINT mcp_server_configs_auth_type_check
    CHECK (auth_type IN ('none', 'oauth2', 'api_key', 'custom_headers', 'user_oidc'));

ALTER TABLE mcp_server_configs
    DROP COLUMN external_auth_provider_id,
    DROP COLUMN tool_rules,
    DROP COLUMN tool_default;

-- No-op for the resource_type value because Postgres cannot remove enum values safely.
