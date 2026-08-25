-- Allow the escalate disposition for MCP gateway tool policy: calls to an
-- escalated tool are held for approval by the sponsoring user before they
-- are forwarded upstream.
ALTER TABLE mcp_server_configs
    DROP CONSTRAINT mcp_server_configs_tool_default_check;

ALTER TABLE mcp_server_configs
    ADD CONSTRAINT mcp_server_configs_tool_default_check
        CHECK (tool_default IN ('enabled', 'disabled', 'escalate'));
