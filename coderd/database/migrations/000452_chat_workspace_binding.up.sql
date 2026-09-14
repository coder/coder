ALTER TABLE chats
    ADD COLUMN build_id UUID REFERENCES workspace_builds(id) ON DELETE SET NULL,
    ADD COLUMN agent_id UUID REFERENCES workspace_agents(id) ON DELETE SET NULL;
