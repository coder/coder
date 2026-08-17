ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:delete';

ALTER TABLE mcp_server_configs
    ADD COLUMN organization_id UUID REFERENCES organizations(id) ON DELETE CASCADE;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM organizations WHERE is_default = true) THEN
        RAISE EXCEPTION 'cannot scope mcp_server_configs: no default organization exists';
    END IF;
END $$;

-- The deployment-wide originals become the default organization's servers,
-- credentials intact. Other organizations start with no MCP servers.
UPDATE mcp_server_configs
SET organization_id = (SELECT id FROM organizations WHERE is_default = true LIMIT 1);

-- Chats outside the default organization referenced deployment-wide configs
-- that now belong to the default organization and no longer resolve in the
-- chat's organization.
UPDATE chats
SET mcp_server_ids = '{}'
WHERE organization_id != (
    SELECT id FROM organizations WHERE is_default = true LIMIT 1
)
    AND cardinality(mcp_server_ids) > 0;

ALTER TABLE mcp_server_configs
    ALTER COLUMN organization_id SET NOT NULL,
    DROP CONSTRAINT mcp_server_configs_slug_key,
    ADD CONSTRAINT mcp_server_configs_organization_id_slug_key UNIQUE (organization_id, slug);

CREATE INDEX idx_mcp_server_configs_organization_id
    ON mcp_server_configs (organization_id);
