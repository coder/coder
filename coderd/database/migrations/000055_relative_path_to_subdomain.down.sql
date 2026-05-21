-- Add column relative_path of type bool to workspace_apps
ALTER TABLE "workspace_apps" ADD COLUMN "relative_path" bool NOT NULL DEFAULT false;

-- Set column relative_path to the opposite of subdomain
UPDATE "workspace_apps" SET "relative_path" = NOT "subdomain";

-- Drop column subdomain
ALTER TABLE "workspace_apps" DROP COLUMN "subdomain";
