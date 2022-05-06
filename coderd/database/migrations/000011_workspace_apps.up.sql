CREATE TABLE workspace_apps (
    id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    agent_id uuid NOT NULL REFERENCES workspace_agents (id) ON DELETE CASCADE,
    name varchar(64) NOT NULL,
    icon varchar(256) NOT NULL,
    -- A command to run when opened.
    command varchar(65534),
    -- A URL or port to target.
    target varchar(65534),
    PRIMARY KEY (id),
    UNIQUE(agent_id, name)
);
