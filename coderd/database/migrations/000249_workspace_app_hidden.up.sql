ALTER TABLE workspace_apps ADD COLUMN hidden boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN workspace_apps.hidden
IS 'Determines if the app is not shown in user interfaces.'
