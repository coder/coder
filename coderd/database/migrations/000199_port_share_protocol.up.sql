CREATE TYPE port_share_protocol AS ENUM ('http', 'https');

ALTER TABLE workspace_agent_port_share
	ADD COLUMN protocol port_share_protocol NOT NULL DEFAULT 'http'::port_share_protocol;
