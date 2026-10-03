-- Configs created outside the default organization cannot move to the
-- deployment-wide table because slugs may collide across organizations.
-- Delete them; the delete trigger from 000510 removes their IDs from chats.
DELETE FROM mcp_server_configs
WHERE organization_id != (
    SELECT id FROM organizations WHERE is_default = true LIMIT 1
);

DROP INDEX idx_mcp_server_configs_organization_id;

ALTER TABLE mcp_server_configs
    DROP CONSTRAINT mcp_server_configs_organization_id_slug_key,
    DROP COLUMN organization_id,
    ADD CONSTRAINT mcp_server_configs_slug_key UNIQUE (slug);

-- Enum values cannot be removed safely from api_key_scope.
