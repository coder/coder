CREATE TABLE jfrog_xray_scans (
	agent_id uuid NOT NULL REFERENCES workspace_agents(id) ON DELETE CASCADE,
	workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	critical integer NOT NULL DEFAULT 0,
	high integer NOT NULL DEFAULT 0,
	medium integer NOT NULL DEFAULT 0,
	results_url text NOT NULL DEFAULT '',
	PRIMARY KEY (agent_id, workspace_id)
);
