CREATE TABLE mcp_gateway_escalations (
    -- The gateway generates the ID so it can correlate held tool calls before
    -- the create round-trip completes.
    id uuid PRIMARY KEY,
    -- The columns below are intentionally not foreign keys: approval audit
    -- history must survive server configuration, identity, and user deletion.
    mcp_server_config_id uuid NOT NULL,
    server_slug text NOT NULL,
    server_url text NOT NULL,
    tool text NOT NULL,
    input jsonb NOT NULL,
    ai_agent_id uuid NOT NULL,
    sponsor_user_id uuid NOT NULL,
    workspace_name text NOT NULL DEFAULT '',
    status text NOT NULL CHECK (status IN ('pending', 'approved', 'denied', 'expired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    resolved_at timestamptz,
    resolved_by uuid
);

COMMENT ON TABLE mcp_gateway_escalations IS
    'MCP tool calls held for sponsor approval. Attribution and server columns are snapshots without foreign keys so audit history survives configuration and identity cleanup.';
COMMENT ON COLUMN mcp_gateway_escalations.mcp_server_config_id IS
    'MCP server configuration snapshot. Not a foreign key; retained after server configuration deletion.';
COMMENT ON COLUMN mcp_gateway_escalations.ai_agent_id IS
    'AI agent identity snapshot. Not a foreign key to ai_agents; retained after identity revocation and cleanup.';
COMMENT ON COLUMN mcp_gateway_escalations.sponsor_user_id IS
    'Sponsoring human user snapshot. Not a foreign key to users; retained after user cleanup.';
COMMENT ON COLUMN mcp_gateway_escalations.resolved_by IS
    'Resolving user snapshot. Not a foreign key to users; retained after user cleanup.';

CREATE INDEX idx_mcp_gateway_escalations_sponsor_user_id_status
    ON mcp_gateway_escalations (sponsor_user_id, status);
CREATE INDEX idx_mcp_gateway_escalations_status_expires_at
    ON mcp_gateway_escalations (status, expires_at);

ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'mcp_gateway_escalation';

ALTER TABLE aibridge_tool_usages
    ADD COLUMN disposition text NOT NULL DEFAULT 'permitted'
        CHECK (disposition IN ('permitted', 'blocked', 'escalated_approved', 'escalated_denied', 'escalated_expired')),
    ADD COLUMN escalation_id uuid;
