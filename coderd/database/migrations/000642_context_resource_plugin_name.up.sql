ALTER TABLE workspace_agent_context_resources
	ADD COLUMN plugin_name TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN workspace_agent_context_resources.plugin_name IS 'Agent Plugin name: the plugin''s own name on an OK plugin row, the owning plugin''s name on a skill or mcp_server row the plugin ships, and empty otherwise.';

ALTER TABLE chat_context_resources
	ADD COLUMN plugin_name TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN chat_context_resources.plugin_name IS 'Agent Plugin name: the plugin''s own name on an OK plugin row, the owning plugin''s name on a skill or mcp_server row the plugin ships, and empty otherwise.';
