BEGIN;

ALTER TABLE workspace_agent_logs SET LOGGED;

-- Revert the workspace_agents table to its former state
ALTER TABLE workspace_agents ADD COLUMN startup_script text;
ALTER TABLE workspace_agents ADD COLUMN startup_script_behavior text;
ALTER TABLE workspace_agents ADD COLUMN shutdown_script_timeout_seconds integer;
ALTER TABLE workspace_agents ADD COLUMN shutdown_script text;
ALTER TABLE workspace_agents ADD COLUMN startup_script_timeout_seconds integer;

-- Reinstate the dropped type
CREATE TYPE workspace_agent_log_source AS ENUM ('startup_script', 'shutdown_script', 'kubernetes_logs', 'envbox', 'envbuilder', 'external');

-- Add old source column back with enum type and drop log_source_id
ALTER TABLE workspace_agent_logs ADD COLUMN source workspace_agent_log_source;
ALTER TABLE workspace_agent_logs DROP COLUMN log_source_id;

-- Drop the newly created tables
DROP TABLE workspace_agent_scripts;
DROP TABLE workspace_agent_log_sources;

COMMIT;
