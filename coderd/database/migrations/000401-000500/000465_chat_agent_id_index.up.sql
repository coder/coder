CREATE INDEX idx_chats_agent_id ON chats(agent_id) WHERE agent_id IS NOT NULL;
