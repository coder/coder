CREATE TYPE workspace_agent_subsystem AS ENUM ('envbuilder', 'envbox', 'none');
ALTER TABLE workspace_agents ADD COLUMN subsystem workspace_agent_subsystem NOT NULL default 'none';
