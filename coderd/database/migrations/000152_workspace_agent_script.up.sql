CREATE TABLE workspace_agent_log_source (
	workspace_agent_id uuid NOT NULL,
	id
	display_name varchar(127) NOT NULL,
	icon text NOT NULL,
);

-- Set the table to unlogged to speed up the inserts
ALTER TABLE workspace_agent_logs SET UNLOGGED;
