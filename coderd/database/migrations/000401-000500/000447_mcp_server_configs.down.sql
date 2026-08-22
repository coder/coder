ALTER TABLE chats DROP COLUMN IF EXISTS mcp_server_ids;
DROP INDEX IF EXISTS idx_mcp_server_configs_enabled;
DROP INDEX IF EXISTS idx_mcp_server_configs_forced;
DROP INDEX IF EXISTS idx_mcp_server_user_tokens_user_id;
DROP TABLE IF EXISTS mcp_server_user_tokens;
DROP TABLE IF EXISTS mcp_server_configs;
