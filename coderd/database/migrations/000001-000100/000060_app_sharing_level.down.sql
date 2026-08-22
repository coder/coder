-- Drop column sharing_level from workspace_apps
ALTER TABLE workspace_apps DROP COLUMN sharing_level;

-- Drop type app_sharing_level
DROP TYPE app_sharing_level;
