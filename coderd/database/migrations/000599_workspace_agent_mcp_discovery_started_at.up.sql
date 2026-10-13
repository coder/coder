ALTER TABLE workspace_agents
    ADD COLUMN mcp_discovery_started_at timestamptz;

COMMENT ON COLUMN workspace_agents.mcp_discovery_started_at IS 'Coder server clock time at which coderd recorded that the agent run''s startup scripts finished (lifecycle ready, start_error, or start_timeout); the agent starts workspace MCP discovery at that moment. NULL while startup scripts run. Chats wait for MCP discovery at most a fixed time after this.';
