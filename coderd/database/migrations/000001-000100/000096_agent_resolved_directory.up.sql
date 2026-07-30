ALTER TABLE ONLY workspace_agents
	ADD COLUMN IF NOT EXISTS expanded_directory varchar(4096) DEFAULT '' NOT NULL;

COMMENT ON COLUMN workspace_agents.expanded_directory
IS 'The resolved path of a user-specified directory. e.g. ~/coder -> /home/coder/coder';
