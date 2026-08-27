-- Escalate rows must be re-narrowed before the constraint can tighten.
UPDATE mcp_server_configs
    SET tool_default = 'disabled'
    WHERE tool_default = 'escalate';

ALTER TABLE mcp_server_configs
    DROP CONSTRAINT mcp_server_configs_tool_default_check;

ALTER TABLE mcp_server_configs
    ADD CONSTRAINT mcp_server_configs_tool_default_check
        CHECK (tool_default IN ('enabled', 'disabled'));
