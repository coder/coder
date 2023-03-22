-- TODO: Do we need an index for workspace_agent_id or is the multi-column PRIMARY
-- key enough?
CREATE TABLE workspace_agent_metadata (
    workspace_agent_id uuid NOT NULL,
    key character varying(128) NOT NULL,
    value text NOT NULL,
    error text NOT NULL,
    timeout bigint NOT NULL,
    interval bigint NOT NULL,
    collected_at timestamp with time zone NOT NULL,
    PRIMARY KEY (workspace_agent_id, key),
    FOREIGN KEY (workspace_agent_id) REFERENCES workspace_agents(id) ON DELETE CASCADE
);
