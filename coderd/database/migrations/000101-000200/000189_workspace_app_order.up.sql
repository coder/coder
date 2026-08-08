ALTER TABLE workspace_apps ADD COLUMN display_order integer NOT NULL DEFAULT 0;

COMMENT ON COLUMN workspace_apps.display_order
IS 'Specifies the order in which to display agent app in user interfaces.';
