-- Rolling this migration back deletes any rows using the user_oidc auth
-- type because they would otherwise violate the restored CHECK constraint.
DELETE FROM mcp_server_configs WHERE auth_type = 'user_oidc';

ALTER TABLE mcp_server_configs
    DROP CONSTRAINT mcp_server_configs_auth_type_check;

ALTER TABLE mcp_server_configs
    ADD CONSTRAINT mcp_server_configs_auth_type_check
    CHECK (auth_type IN ('none', 'oauth2', 'api_key', 'custom_headers'));
