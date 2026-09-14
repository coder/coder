ALTER TABLE workspace_agents
ADD COLUMN parent_id UUID REFERENCES workspace_agents (id) ON DELETE CASCADE;
