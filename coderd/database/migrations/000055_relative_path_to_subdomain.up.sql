-- Add column subdomain of type bool to workspace_apps
ALTER TABLE "workspace_apps" ADD COLUMN "subdomain" bool NOT NULL DEFAULT false;

-- Set column subdomain to the opposite of relative_path
UPDATE "workspace_apps" SET "subdomain" = NOT "relative_path";

-- Drop column relative_path
ALTER TABLE "workspace_apps" DROP COLUMN "relative_path";
