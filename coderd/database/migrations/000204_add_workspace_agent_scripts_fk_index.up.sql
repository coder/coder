CREATE INDEX workspace_agent_scripts_workspace_agent_id_idx ON workspace_agent_scripts (workspace_agent_id);

COMMENT ON INDEX workspace_agent_scripts_workspace_agent_id_idx IS 'Foreign key support index for faster lookups';
