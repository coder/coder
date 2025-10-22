ALTER TABLE workspace_agent_metadata ADD COLUMN display_order integer NOT NULL DEFAULT 0;

COMMENT ON COLUMN workspace_agent_metadata.display_order
IS 'Specifies the order in which to display agent metadata in user interfaces.';
