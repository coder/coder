-- This table is UNLOGGED because it is very update-heavy and the the data 
-- is not valuable enough to justify the overhead of WAL logging. This should
-- give us a ~70% improvement in write throughput.
CREATE UNLOGGED TABLE workspace_agent_metadata (
    workspace_agent_id uuid NOT NULL,
    display_name varchar(127) NOT NULL,
    key varchar(127) NOT NULL,
    script varchar(65535) NOT NULL,
    value varchar(65535) NOT NULL DEFAULT '',
    error varchar(65535) NOT NULL DEFAULT '',
    timeout bigint NOT NULL,
    interval bigint NOT NULL,
    collected_at timestamp with time zone NOT NULL DEFAULT '0001-01-01 00:00:00+00',
    PRIMARY KEY (workspace_agent_id, key),
    FOREIGN KEY (workspace_agent_id) REFERENCES workspace_agents(id) ON DELETE CASCADE
);
