BEGIN;
ALTER TABLE workspace_agents DROP COLUMN display_apps;
DROP TYPE display_app; 
COMMIT;

