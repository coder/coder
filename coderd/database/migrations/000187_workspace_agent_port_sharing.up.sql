CREATE TABLE workspace_agent_port_sharing (
	workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
	agent_name text NOT NULL,
	port integer NOT NULL,
	share_level integer NOT NULL
);
