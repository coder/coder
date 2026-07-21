ALTER TABLE mcp_server_configs
    ADD COLUMN user_acl jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN group_acl jsonb NOT NULL DEFAULT '{}'::jsonb;

UPDATE mcp_server_configs
SET group_acl = jsonb_build_object(organization_id::text, jsonb_build_array('read'));

ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:share';
