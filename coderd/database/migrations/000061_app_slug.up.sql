BEGIN;

-- add "slug" column to "workspace_apps" table
ALTER TABLE "workspace_apps" ADD COLUMN "slug" text DEFAULT '';

-- copy the "name" column for each workspace app to the "slug" column
UPDATE "workspace_apps" SET "slug" = "name";

-- make "slug" column not nullable and remove default
ALTER TABLE "workspace_apps" ALTER COLUMN "slug" SET NOT NULL;
ALTER TABLE "workspace_apps" ALTER COLUMN "slug" DROP DEFAULT;

COMMIT;
