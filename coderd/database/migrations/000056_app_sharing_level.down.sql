-- Drop column share_level from workspace_apps
ALTER TABLE workspace_apps DROP COLUMN share_level;

-- Drop type app_share_level
DROP TYPE app_share_level;
