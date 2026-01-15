ALTER TABLE workspace_agent_devcontainers
	ADD COLUMN subagent_id UUID REFERENCES workspace_agents(id) ON DELETE SET NULL;
