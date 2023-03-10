CREATE TABLE IF NOT EXISTS workspace_agent_startup_logs (
    agent_id uuid NOT NULL REFERENCES workspace_agents (id) ON DELETE CASCADE,
	id bigint NOT NULL,
    created_at timestamptz NOT NULL,
    output varchar(1024) NOT NULL,
    UNIQUE(agent_id, id)
);

CREATE INDEX workspace_agent_startup_logs_id_agent_id_idx ON workspace_agent_startup_logs USING btree (agent_id, id ASC);
