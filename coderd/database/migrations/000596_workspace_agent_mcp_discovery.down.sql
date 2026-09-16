ALTER TABLE workspace_agent_context_snapshots
    DROP COLUMN mcp_discovery_phase,
    DROP COLUMN agent_run_id;

DROP TYPE workspace_agent_mcp_discovery_phase;

ALTER TABLE workspace_agents
    DROP COLUMN agent_run_id;
