ALTER TABLE workspace_agents ADD COLUMN display_order integer NOT NULL DEFAULT 0;

COMMENT ON COLUMN workspace_agents.display_order
IS 'Specifies the order in which to display agents in user interfaces.';
