CREATE TABLE agent_stats (
	id uuid NOT NULL,
	PRIMARY KEY (id),
    created_at timestamptz NOT NULL,
    user_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
	payload jsonb NOT NULL
);

-- We use created_at for DAU analysis and pruning.
CREATE INDEX idx_agent_stats_created_at ON agent_stats USING btree (created_at);

-- We perform user grouping to analyze DAUs.
CREATE INDEX idx_agent_stats_user_id ON agent_stats USING btree (user_id);
