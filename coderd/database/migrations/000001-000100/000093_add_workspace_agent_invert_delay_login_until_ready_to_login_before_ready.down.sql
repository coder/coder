ALTER TABLE workspace_agents RENAME COLUMN login_before_ready TO delay_login_until_ready;
ALTER TABLE workspace_agents ALTER COLUMN delay_login_until_ready SET DEFAULT false;

UPDATE workspace_agents SET delay_login_until_ready = NOT delay_login_until_ready;

COMMENT ON COLUMN workspace_agents.delay_login_until_ready IS 'If true, the agent will delay logins until it is ready (e.g. executing startup script has ended).';
