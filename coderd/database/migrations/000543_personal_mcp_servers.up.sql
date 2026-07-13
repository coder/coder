-- Personal MCP servers: a NULL owner_id means the config is
-- deployment-wide (admin-managed); a non-NULL owner_id means the config
-- is personal to that user and invisible to everyone else.
ALTER TABLE mcp_server_configs
    ADD COLUMN owner_id uuid REFERENCES users (id) ON DELETE CASCADE;

COMMENT ON COLUMN mcp_server_configs.owner_id IS 'When set, the MCP server config is personal to this user and hidden from all other users. NULL means deployment-wide.';

-- Replace the global slug uniqueness with per-scope uniqueness so a
-- personal slug cannot collide with (or block) global slugs or other
-- users' personal slugs.
ALTER TABLE mcp_server_configs
    DROP CONSTRAINT mcp_server_configs_slug_key;

CREATE UNIQUE INDEX idx_mcp_server_configs_global_slug ON mcp_server_configs (slug) WHERE owner_id IS NULL;

CREATE UNIQUE INDEX idx_mcp_server_configs_owner_slug ON mcp_server_configs (owner_id, slug) WHERE owner_id IS NOT NULL;

CREATE INDEX idx_mcp_server_configs_owner_id ON mcp_server_configs (owner_id) WHERE owner_id IS NOT NULL;
