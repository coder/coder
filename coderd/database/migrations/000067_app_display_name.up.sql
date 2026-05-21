-- rename column "name" to "display_name" on "workspace_apps"
ALTER TABLE "workspace_apps" RENAME COLUMN "name" TO "display_name";

-- drop constraint "workspace_apps_agent_id_name_key" on "workspace_apps".
ALTER TABLE ONLY workspace_apps DROP CONSTRAINT IF EXISTS workspace_apps_agent_id_name_key;
