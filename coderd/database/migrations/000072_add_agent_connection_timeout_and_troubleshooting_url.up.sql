BEGIN;

-- Default value same as in terraform-provider-coder.
ALTER TABLE workspace_agents
	ADD COLUMN connection_timeout integer NOT NULL DEFAULT 120;

ALTER TABLE workspace_agents
	ADD COLUMN troubleshooting_url text NOT NULL DEFAULT '';

COMMIT;
