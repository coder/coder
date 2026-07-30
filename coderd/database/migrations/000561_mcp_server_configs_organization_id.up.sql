-- Org-scope MCP server configs: every config belongs to exactly one
-- organization. This migration backfills all existing rows to the default
-- organization; runtime behavior is preserved by a chat-org-then-default-org
-- lookup window that ends at the org-scoping cutover (CODAGT-711 B3).

-- Step 1: Add the nullable column with FK (000467 recipe).
ALTER TABLE mcp_server_configs
    ADD COLUMN organization_id UUID REFERENCES organizations(id) ON DELETE CASCADE;

-- Step 2: Backfill every row to the default organization. Abort loudly if
-- the deployment has no default organization; a silent partial backfill
-- would fail the NOT NULL step with an opaque error.
DO $$
DECLARE
    default_org_id UUID;
BEGIN
    SELECT id INTO default_org_id FROM organizations WHERE is_default = true LIMIT 1;
    IF default_org_id IS NULL THEN
        RAISE EXCEPTION 'cannot backfill mcp_server_configs.organization_id: no default organization exists';
    END IF;
    UPDATE mcp_server_configs SET organization_id = default_org_id;
END $$;

-- Step 3: Enforce NOT NULL going forward.
ALTER TABLE mcp_server_configs ALTER COLUMN organization_id SET NOT NULL;

-- Step 4: Slug uniqueness becomes per-organization.
ALTER TABLE mcp_server_configs DROP CONSTRAINT mcp_server_configs_slug_key;
ALTER TABLE mcp_server_configs ADD CONSTRAINT mcp_server_configs_organization_id_slug_key UNIQUE (organization_id, slug);

-- Step 5: Index for efficient lookups by organization.
CREATE INDEX idx_mcp_server_configs_organization_id ON mcp_server_configs (organization_id);
