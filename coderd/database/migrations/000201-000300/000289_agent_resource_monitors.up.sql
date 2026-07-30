CREATE TABLE workspace_agent_memory_resource_monitors (
	agent_id 	uuid NOT NULL REFERENCES workspace_agents(id) ON DELETE CASCADE,
	enabled 	boolean 					NOT NULL,
	threshold 	integer 					NOT NULL,
	created_at 	timestamp with time zone 	NOT NULL,
	PRIMARY KEY (agent_id)
);

CREATE TABLE workspace_agent_volume_resource_monitors (
	agent_id 	uuid NOT NULL REFERENCES workspace_agents(id) ON DELETE CASCADE,
	enabled 	boolean 					NOT NULL,
	threshold 	integer 					NOT NULL,
	path 		text 						NOT NULL,
	created_at 	timestamp with time zone 	NOT NULL,
	PRIMARY KEY (agent_id, path)
);
