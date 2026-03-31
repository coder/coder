-- Add group scoping to chat model configs and MCP server configs.
-- Empty array means "available to all users" (backward compatible).
-- Non-empty means "only members of these groups can see the resource."

ALTER TABLE chat_model_configs
    ADD COLUMN allowed_group_ids uuid[] NOT NULL DEFAULT '{}';

ALTER TABLE mcp_server_configs
    ADD COLUMN allowed_group_ids uuid[] NOT NULL DEFAULT '{}';
