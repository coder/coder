ALTER TABLE workspace_agents RENAME COLUMN delay_login_until_ready TO login_before_ready;
ALTER TABLE workspace_agents ALTER COLUMN login_before_ready SET DEFAULT true;

UPDATE workspace_agents SET login_before_ready = NOT login_before_ready;

COMMENT ON COLUMN workspace_agents.login_before_ready IS 'If true, the agent will not prevent login before it is ready (e.g. startup script is still executing).';
