CREATE TABLE workspace_agent_memory_resource_monitors (
	agent_id 	uuid 						NOT NULL,
	enabled 	boolean 					NOT NULL,
	threshold 	integer 					NOT NULL,
	created_at 	timestamp with time zone 	NOT NULL
);

CREATE TABLE workspace_agent_volume_resource_monitors (
	agent_id 	uuid 						NOT NULL,
	enabled 	boolean 					NOT NULL,
	threshold 	integer 					NOT NULL,
	path 		text 						NOT NULL,
	created_at 	timestamp with time zone 	NOT NULL
);
