BEGIN;
CREATE TABLE workspace_agent_log_source (
	workspace_agent_id uuid NOT NULL,
	id uuid NOT NULL,
	created_at timestamptz NOT NULL,
	display_name varchar(127) NOT NULL,
	icon text NOT NULL,
	PRIMARY KEY (workspace_agent_id, id)
);

CREATE TABLE workspace_agent_script (
	workspace_agent_id uuid NOT NULL,
	log_source_id uuid NOT NULL,
	created_at timestamptz NOT NULL,
	script text NOT NULL,
	schedule text NOT NULL,
	login_before_ready boolean NOT NULL,
	name varchar(127) NOT NULL,
	description text NOT NULL,
	PRIMARY KEY (workspace_agent_id, id)
);

-- Set the table to unlogged to speed up the inserts
ALTER TABLE workspace_agent_logs SET UNLOGGED;
COMMIT;
