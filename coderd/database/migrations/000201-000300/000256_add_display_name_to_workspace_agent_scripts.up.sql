ALTER TABLE workspace_agent_scripts ADD COLUMN display_name text;

UPDATE workspace_agent_scripts
	SET display_name = workspace_agent_log_sources.display_name
FROM workspace_agent_log_sources
	WHERE workspace_agent_scripts.log_source_id = workspace_agent_log_sources.id;

ALTER TABLE workspace_agent_scripts ALTER COLUMN display_name SET NOT NULL;
