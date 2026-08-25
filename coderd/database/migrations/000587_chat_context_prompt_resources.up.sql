-- MCP resources describe the bound agent's live runtime capabilities and are
-- read from workspace_agent_context_resources. Chat pins retain only prompt
-- context that must survive agent replacement and workspace rebuilds.
DELETE FROM chat_context_resources
WHERE body_kind IN ('mcp_config', 'mcp_server');

COMMENT ON TABLE chat_context_resources IS 'Per-chat pinned prompt context a chat is hydrated against. Instruction files and skills are copied from workspace_agent_context_resources at chat hydration and context refresh; the pin survives agent replacement and workspace rebuilds.';
COMMENT ON COLUMN chat_context_resources.source IS 'Canonical source path for a pinned prompt resource.';
COMMENT ON COLUMN chat_context_resources.body_kind IS 'Discriminator for the pinned prompt body JSON shape. Currently instruction_file and skill; PLUGIN/HOOK/SUBAGENT/COMMAND are reserved for the Claude Code plugin RFC.';
COMMENT ON COLUMN chat_context_resources.content_hash IS 'sha256 over the pinned prompt resource original bytes.';
