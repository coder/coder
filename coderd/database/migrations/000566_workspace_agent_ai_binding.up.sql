ALTER TABLE workspace_agents
	ADD COLUMN ai_agent_id uuid REFERENCES ai_agents(user_id);

CREATE INDEX workspace_agents_ai_agent_id_idx
	ON workspace_agents (ai_agent_id);
