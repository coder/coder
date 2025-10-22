ALTER TABLE workspace_agents ADD COLUMN login_before_ready boolean NOT NULL DEFAULT TRUE;

UPDATE workspace_agents SET login_before_ready = CASE WHEN startup_script_behavior = 'non-blocking' THEN TRUE ELSE FALSE END;

ALTER TABLE workspace_agents DROP COLUMN startup_script_behavior;
DROP TYPE startup_script_behavior;

COMMENT ON COLUMN workspace_agents.login_before_ready IS 'If true, the agent will delay logins until it is ready (e.g. executing startup script has ended).';
