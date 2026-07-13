-- Personal rows must be removed before restoring the global slug
-- uniqueness constraint, since a personal slug may duplicate a global
-- one.
DELETE FROM mcp_server_configs WHERE owner_id IS NOT NULL;

DROP INDEX idx_mcp_server_configs_owner_id;

DROP INDEX idx_mcp_server_configs_owner_slug;

DROP INDEX idx_mcp_server_configs_global_slug;

ALTER TABLE mcp_server_configs
    DROP COLUMN owner_id;

ALTER TABLE mcp_server_configs
    ADD CONSTRAINT mcp_server_configs_slug_key UNIQUE (slug);
