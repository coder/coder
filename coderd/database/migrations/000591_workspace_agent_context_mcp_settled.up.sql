ALTER TABLE workspace_agent_context_snapshots
    ADD COLUMN mcp_settled boolean;

COMMENT ON COLUMN workspace_agent_context_snapshots.mcp_settled IS
    'True once the agent''s first MCP registration attempt settled (success, failure, or zero servers). NULL for legacy agents or before the first settled push.';
