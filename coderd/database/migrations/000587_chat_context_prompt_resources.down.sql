-- MCP rows are derived from live workspace agent state and cannot be restored.
-- Older code repopulates them on subsequent chat hydration or context refresh.
COMMENT ON TABLE chat_context_resources IS 'Per-chat pinned copy of the agent context resources a chat is hydrated against. Copied from workspace_agent_context_resources at chat hydration and context refresh; survives agent replacement and workspace rebuilds.';
COMMENT ON COLUMN chat_context_resources.source IS 'Resource locator: canonical file path for file-backed kinds, or the MCP server name for mcp_server resources.';
COMMENT ON COLUMN chat_context_resources.body_kind IS 'Discriminator for the body JSON shape. Matches the proto oneof variant: instruction_file, skill, mcp_config, mcp_server. PLUGIN/HOOK/SUBAGENT/COMMAND are reserved for the Claude Code plugin RFC.';
COMMENT ON COLUMN chat_context_resources.content_hash IS 'sha256 over the resource''s original bytes (or transport-encoded server tool list).';
