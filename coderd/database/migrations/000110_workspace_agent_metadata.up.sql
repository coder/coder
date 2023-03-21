-- TODO: Do we need an index for workspace_agent_id or is the multi-column PRIMARY
-- key enough?
CREATE TABLE workspace_agent_metadata (
    workspace_id uuid NOT NULL,
    workspace_agent_id uuid NOT NULL,
    key character varying(128) NOT NULL,
    value text NOT NULL,
    error text NOT NULL,
    collected_at timestamp with time zone NOT NULL,
    PRIMARY KEY (workspace_agent_id, key),
    FOREIGN KEY (workspace_agent_id) REFERENCES workspace_agents(id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);
