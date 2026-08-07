ALTER TABLE mcp_server_configs
    ADD COLUMN group_acl JSONB NOT NULL DEFAULT '{}'::JSONB,
    ADD COLUMN user_acl JSONB NOT NULL DEFAULT '{}'::JSONB,
    ADD CONSTRAINT mcp_server_configs_group_acl_is_object CHECK (jsonb_typeof(group_acl) = 'object'),
    ADD CONSTRAINT mcp_server_configs_user_acl_is_object CHECK (jsonb_typeof(user_acl) = 'object');

UPDATE mcp_server_configs
SET group_acl = jsonb_build_object(
    organization_id::text,
    jsonb_build_object('permissions', jsonb_build_array('read'))
);
