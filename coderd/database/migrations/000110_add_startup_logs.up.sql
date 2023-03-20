CREATE TABLE IF NOT EXISTS workspace_agent_startup_logs (
    agent_id uuid NOT NULL REFERENCES workspace_agents (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    output varchar(1024) NOT NULL,
	id BIGSERIAL PRIMARY KEY
);
CREATE INDEX workspace_agent_startup_logs_id_agent_id_idx ON workspace_agent_startup_logs USING btree (agent_id, id ASC);

-- The maximum length of startup logs is 1MB per workspace agent.
ALTER TABLE workspace_agents ADD COLUMN startup_logs_length integer NOT NULL DEFAULT 0 CONSTRAINT max_startup_logs_length CHECK (startup_logs_length <= 1048576);
ALTER TABLE workspace_agents ADD COLUMN startup_logs_overflowed boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN workspace_agents.startup_logs_length IS 'Total length of startup logs';
COMMENT ON COLUMN workspace_agents.startup_logs_overflowed IS 'Whether the startup logs overflowed in length';
