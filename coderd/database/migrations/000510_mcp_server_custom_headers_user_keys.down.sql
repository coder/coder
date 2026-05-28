DROP INDEX IF EXISTS idx_mcp_server_user_header_values_user_id;
DROP TABLE IF EXISTS mcp_server_user_header_values;

ALTER TABLE mcp_server_configs
    DROP COLUMN IF EXISTS custom_headers_user_keys;
