ALTER TABLE workspace_agents
	ADD COLUMN motd_file text NOT NULL DEFAULT '';

COMMENT ON COLUMN workspace_agents.motd_file IS 'Path to file inside workspace containing the message of the day (MOTD) to show to the user when logging in via SSH.';
