CREATE TABLE workspace_agent_devcontainers (
	id UUID PRIMARY KEY,
	workspace_agent_id UUID NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	workspace_folder TEXT NOT NULL,
	config_path TEXT NOT NULL,
	FOREIGN KEY (workspace_agent_id) REFERENCES workspace_agents(id) ON DELETE CASCADE
);

COMMENT ON TABLE workspace_agent_devcontainers IS 'Workspace agent devcontainer configuration';
COMMENT ON COLUMN workspace_agent_devcontainers.id IS 'Unique identifier';
COMMENT ON COLUMN workspace_agent_devcontainers.workspace_agent_id IS 'Workspace agent foreign key';
COMMENT ON COLUMN workspace_agent_devcontainers.created_at IS 'Creation timestamp';
COMMENT ON COLUMN workspace_agent_devcontainers.workspace_folder IS 'Workspace folder';
COMMENT ON COLUMN workspace_agent_devcontainers.config_path IS 'Path to devcontainer.json.';
