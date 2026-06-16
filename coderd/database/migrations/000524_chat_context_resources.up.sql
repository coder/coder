-- Creates chat_context_resources: a per-chat pinned copy of
-- workspace_agent_context_resources (semantics in COMMENT ON TABLE
-- below). Migration-specific notes: there is deliberately no FK to
-- workspace_agents so the pin survives agent replacement and workspace
-- rebuilds, and the body_kind/status enum types are reused from 000522
-- and must not be recreated here.
CREATE TABLE chat_context_resources (
    chat_id UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    source TEXT NOT NULL,
    body_kind workspace_agent_context_body_kind NOT NULL,
    body JSONB NOT NULL,
    content_hash BYTEA NOT NULL,
    size_bytes BIGINT NOT NULL,
    status workspace_agent_context_resource_status NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    source_path TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (chat_id, source)
);

COMMENT ON TABLE chat_context_resources IS 'Per-chat pinned copy of the agent context resources a chat is hydrated against. Copied from workspace_agent_context_resources at chat hydration and context refresh; survives agent replacement and workspace rebuilds.';
COMMENT ON COLUMN chat_context_resources.source IS 'Resource locator: canonical file path for file-backed kinds, or the MCP server name for mcp_server resources.';
COMMENT ON COLUMN chat_context_resources.body_kind IS 'Discriminator for the body JSON shape. Matches the proto oneof variant: instruction_file, skill, mcp_config, mcp_server. PLUGIN/HOOK/SUBAGENT/COMMAND are reserved for the Claude Code plugin RFC.';
COMMENT ON COLUMN chat_context_resources.body IS 'protojson-encoded variant body matching body_kind. Always populated; non-OK statuses use the variant zero value so the wire kind is still attributable.';
COMMENT ON COLUMN chat_context_resources.content_hash IS 'sha256 over the resource''s original bytes (or transport-encoded server tool list).';
COMMENT ON COLUMN chat_context_resources.size_bytes IS 'Original payload size in bytes; populated regardless of status.';
COMMENT ON COLUMN chat_context_resources.status IS 'Per-resource status. ok carries a populated body; oversize, unreadable, invalid, and excluded carry an empty body plus an error string.';
COMMENT ON COLUMN chat_context_resources.error IS 'Per-resource error or warning string. Populated whenever status is non-ok; may also carry a non-fatal warning when status is ok.';
COMMENT ON COLUMN chat_context_resources.source_path IS 'User-declared scan root that produced this resource. Empty for built-in scan roots.';
