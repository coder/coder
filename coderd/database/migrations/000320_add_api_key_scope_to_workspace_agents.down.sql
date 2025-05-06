-- Remove the api_key_scope column from the workspace_agents table
ALTER TABLE workspace_agents
DROP COLUMN IF EXISTS api_key_scope;

-- Drop the enum type for API key scope
DROP TYPE IF EXISTS api_key_scope_enum;
