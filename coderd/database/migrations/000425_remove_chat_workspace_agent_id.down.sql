ALTER TABLE chats ADD COLUMN workspace_agent_id UUID REFERENCES workspace_agents(id) ON DELETE SET NULL;
