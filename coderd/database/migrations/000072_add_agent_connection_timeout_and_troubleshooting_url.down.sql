BEGIN;

ALTER TABLE workspace_agents
	DROP COLUMN connection_timeout;

ALTER TABLE workspace_agents
	DROP COLUMN troubleshooting_url;

COMMIT;
