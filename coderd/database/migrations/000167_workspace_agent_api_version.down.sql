BEGIN;
ALTER TABLE workspace_agents DROP COLUMN api_version;
COMMIT;
