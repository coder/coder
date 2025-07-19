-- Add workspace_apps column to workspace_agents table
ALTER TABLE workspace_agents ADD COLUMN workspace_apps text[] DEFAULT NULL;

COMMENT ON COLUMN workspace_agents.workspace_apps IS 'List of app IDs to display in the workspace table, configured via terraform';
