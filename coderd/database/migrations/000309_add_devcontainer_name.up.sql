ALTER TABLE workspace_agent_devcontainers ADD COLUMN name TEXT NOT NULL DEFAULT '';
ALTER TABLE workspace_agent_devcontainers ALTER COLUMN name DROP DEFAULT;
