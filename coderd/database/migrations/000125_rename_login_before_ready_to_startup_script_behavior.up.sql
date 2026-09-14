CREATE TYPE startup_script_behavior AS ENUM ('blocking', 'non-blocking');
ALTER TABLE workspace_agents ADD COLUMN startup_script_behavior startup_script_behavior NOT NULL DEFAULT 'non-blocking';

UPDATE workspace_agents SET startup_script_behavior = (CASE WHEN login_before_ready THEN 'non-blocking' ELSE 'blocking' END)::startup_script_behavior;

ALTER TABLE workspace_agents DROP COLUMN login_before_ready;

COMMENT ON COLUMN workspace_agents.startup_script_behavior IS 'When startup script behavior is non-blocking, the workspace will be ready and accessible upon agent connection, when it is blocking, workspace will wait for the startup script to complete before becoming ready and accessible.';
