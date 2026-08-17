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

-- Pre-scoping replicas resolve configs globally and can write another org's
-- config ID into a chat during a rolling upgrade. Drop those foreign IDs
-- (keeping unknown IDs as written); remove once those replicas are gone.
CREATE FUNCTION drop_cross_org_chat_mcp_server_ids()
    RETURNS TRIGGER AS
$$
BEGIN
    IF NEW.mcp_server_ids IS NULL OR cardinality(NEW.mcp_server_ids) = 0 THEN
        RETURN NEW;
    END IF;
    SELECT COALESCE(
        array_agg(item.config_id ORDER BY item.position),
        '{}'::uuid[]
    )
    INTO NEW.mcp_server_ids
    FROM unnest(NEW.mcp_server_ids) WITH ORDINALITY AS item(config_id, position)
    LEFT JOIN mcp_server_configs AS config ON config.id = item.config_id
    WHERE config.id IS NULL
        OR config.organization_id = NEW.organization_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER drop_cross_org_chat_mcp_server_ids
    BEFORE INSERT OR UPDATE OF mcp_server_ids ON chats FOR EACH ROW
    EXECUTE PROCEDURE drop_cross_org_chat_mcp_server_ids();

COMMENT ON TRIGGER drop_cross_org_chat_mcp_server_ids ON chats IS
    'Rolling-upgrade compatibility: drops config IDs written by pre-organization-scoping replicas that resolve to another organization''s config.';
