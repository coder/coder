ALTER TABLE ONLY workspace_agents ADD COLUMN version text DEFAULT ''::text NOT NULL;
COMMENT ON COLUMN workspace_agents.version IS 'version tracks the version of the currently running workspace agent. Workspace agents register their version upon start.';
