CREATE TABLE workspace_agent_plugins (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    agent_id UUID NOT NULL REFERENCES workspace_agents(id) ON DELETE CASCADE,
    slug VARCHAR(64) NOT NULL,
    display_name VARCHAR(256) NOT NULL DEFAULT '',
    icon VARCHAR(256) NOT NULL DEFAULT '',
    url VARCHAR(4096) NOT NULL,
    backend_entry VARCHAR(1024) NOT NULL DEFAULT '',
    UNIQUE (agent_id, slug)
);
