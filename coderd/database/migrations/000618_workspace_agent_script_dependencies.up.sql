ALTER TABLE workspace_agent_scripts
	ADD COLUMN resource_address TEXT NOT NULL DEFAULT '',
	ADD COLUMN dependencies JSONB NOT NULL DEFAULT '[]'::jsonb,
	ADD CONSTRAINT workspace_agent_scripts_dependencies_is_array
		CHECK (jsonb_typeof(dependencies) = 'array');
