ALTER TABLE workspace_agents
    ADD COLUMN agent_run_id text NOT NULL DEFAULT '';

COMMENT ON COLUMN workspace_agents.agent_run_id IS 'UUID the agent process generates once and reports in Startup. Stable across RPC reconnects, changes on process restart, empty for agents that predate the field. A context snapshot with the same agent_run_id was published by the current process.';

CREATE TYPE workspace_agent_mcp_discovery_phase AS ENUM (
    'unspecified',
    'pending',
    'complete'
);

ALTER TABLE workspace_agent_context_snapshots
    ADD COLUMN agent_run_id text NOT NULL DEFAULT '',
    ADD COLUMN mcp_discovery_phase workspace_agent_mcp_discovery_phase NOT NULL DEFAULT 'unspecified';

COMMENT ON COLUMN workspace_agent_context_snapshots.agent_run_id IS 'agent_run_id of the agent process that pushed this snapshot. Compared with workspace_agents.agent_run_id to tell whether the snapshot describes the current process. Empty for legacy agents.';

COMMENT ON COLUMN workspace_agent_context_snapshots.mcp_discovery_phase IS 'Workspace MCP discovery completeness for the pushing process: unspecified (legacy agent, no guarantee), pending (initial reload not finished), complete (initial reload reached a terminal result before this snapshot).';
