CREATE TABLE jfrog_xray_scans (
	agent_id uuid NOT NULL PRIMARY KEY REFERENCES workspace_agents(id) ON DELETE CASCADE,
	workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	payload jsonb NOT NULL DEFAULT '{}'
);

ALTER TABLE jfrog_xray_scans ADD CONSTRAINT jfrog_xray_scans_workspace_id_agent_id UNIQUE (agent_id, workspace_id);
