-- Discriminator for the body JSON shape stored with each context
-- resource. Matches the proto oneof variant names. plugin, hook,
-- subagent, and command are reserved for the Claude Code plugin RFC.
CREATE TYPE workspace_agent_context_body_kind AS ENUM (
    'instruction_file',
    'skill',
    'mcp_config',
    'mcp_server',
    'plugin',
    'hook',
    'subagent',
    'command'
);

-- Per-resource resolution status reported by the agent.
CREATE TYPE workspace_agent_context_resource_status AS ENUM (
    'ok',
    'oversize',
    'unreadable',
    'invalid',
    'excluded'
);

-- Latest workspace agent context snapshot, one row per agent.
-- Overwritten on each PushContextState; no history.
CREATE TABLE workspace_agent_context_snapshots (
    workspace_agent_id UUID PRIMARY KEY REFERENCES workspace_agents(id) ON DELETE CASCADE,
    version BIGINT NOT NULL,
    aggregate_hash BYTEA NOT NULL,
    snapshot_error TEXT NOT NULL DEFAULT '',
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE workspace_agent_context_snapshots IS 'Latest workspace agent context snapshot received via PushContextState. One row per workspace agent, overwritten in place.';
COMMENT ON COLUMN workspace_agent_context_snapshots.version IS 'Monotonic per-agent-process push counter. Resets to one when the agent process restarts; combined with the initial flag on the wire to detect agent reboots.';
COMMENT ON COLUMN workspace_agent_context_snapshots.aggregate_hash IS 'sha256 over a canonical encoding of every resource in the snapshot. Identical inputs always produce identical hashes; chat hydration uses this to detect drift.';
COMMENT ON COLUMN workspace_agent_context_snapshots.snapshot_error IS 'Singular snapshot-level error string (count cap exceeded, watcher degraded, etc.). Empty when healthy.';
COMMENT ON COLUMN workspace_agent_context_snapshots.received_at IS 'Time at which coderd received the push.';

-- Resolved resources within a snapshot. Keyed by (agent, source); a
-- subsequent push upserts known sources and the agentapi handler
-- deletes any sources absent from the latest push in the same
-- transaction.
CREATE TABLE workspace_agent_context_resources (
    workspace_agent_id UUID NOT NULL REFERENCES workspace_agents(id) ON DELETE CASCADE,
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
    PRIMARY KEY (workspace_agent_id, source)
);

COMMENT ON TABLE workspace_agent_context_resources IS 'Per-resource state for the latest pushed workspace agent context snapshot.';
COMMENT ON COLUMN workspace_agent_context_resources.source IS 'Resource locator: canonical file path for file-backed kinds, or the MCP server name for mcp_server resources.';
COMMENT ON COLUMN workspace_agent_context_resources.body_kind IS 'Discriminator for the body JSON shape. Matches the proto oneof variant: instruction_file, skill, mcp_config, mcp_server. PLUGIN/HOOK/SUBAGENT/COMMAND are reserved for the Claude Code plugin RFC.';
COMMENT ON COLUMN workspace_agent_context_resources.body IS 'protojson-encoded variant body matching body_kind. Always populated; non-OK statuses use the variant zero value so the wire kind is still attributable.';
COMMENT ON COLUMN workspace_agent_context_resources.content_hash IS 'sha256 over the resource''s original bytes (or transport-encoded server tool list).';
COMMENT ON COLUMN workspace_agent_context_resources.size_bytes IS 'Original payload size in bytes; populated regardless of status.';
COMMENT ON COLUMN workspace_agent_context_resources.status IS 'Per-resource status. ok carries a populated body; oversize, unreadable, invalid, and excluded carry an empty body plus an error string.';
COMMENT ON COLUMN workspace_agent_context_resources.error IS 'Per-resource error or warning string. Populated whenever status is non-ok; may also carry a non-fatal warning when status is ok.';
COMMENT ON COLUMN workspace_agent_context_resources.source_path IS 'User-declared scan root that produced this resource. Empty for built-in scan roots.';
