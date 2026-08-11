ALTER TABLE mcp_server_configs
    DROP COLUMN group_acl,
    DROP COLUMN user_acl;

-- Enum values cannot be removed safely from api_key_scope, so the
-- share scope added by the up migration stays.
