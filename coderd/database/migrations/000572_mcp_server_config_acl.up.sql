ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:share';

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

-- Preserve pre-ACL replica inserts during rolling upgrades by granting
-- Everyone read access when both ACL columns keep their empty defaults.
CREATE FUNCTION default_mcp_server_config_acl() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.group_acl = '{}'::jsonb AND NEW.user_acl = '{}'::jsonb THEN
        NEW.group_acl := jsonb_build_object(
            NEW.organization_id::text,
            jsonb_build_object('permissions', jsonb_build_array('read'))
        );
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER default_mcp_server_config_acl
    BEFORE INSERT ON mcp_server_configs
    FOR EACH ROW
    EXECUTE FUNCTION default_mcp_server_config_acl();
