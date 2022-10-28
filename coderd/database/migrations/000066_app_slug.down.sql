-- drop unique index on "slug" column
ALTER TABLE "workspace_apps" DROP CONSTRAINT IF EXISTS "workspace_apps_agent_id_slug_idx";

-- drop "slug" column from "workspace_apps" table
ALTER TABLE "workspace_apps" DROP COLUMN "slug";
