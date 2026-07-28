-- Restore the deployment-wide slug uniqueness before dropping the org
-- column. This is safe only because every row lives in the default
-- organization during the schema-stage window; the org-scoping cutover
-- (CODAGT-711 B3) removes this assumption.
ALTER TABLE mcp_server_configs DROP CONSTRAINT mcp_server_configs_organization_id_slug_key;
ALTER TABLE mcp_server_configs ADD CONSTRAINT mcp_server_configs_slug_key UNIQUE (slug);

DROP INDEX idx_mcp_server_configs_organization_id;

ALTER TABLE mcp_server_configs DROP COLUMN organization_id;
