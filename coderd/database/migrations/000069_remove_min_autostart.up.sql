-- drop "min_autostart_interval" column from "templates" table
ALTER TABLE "templates" DROP COLUMN "min_autostart_interval";

-- rename "max_ttl" to "default_ttl" on "templates" table
ALTER TABLE "templates" RENAME COLUMN "max_ttl" TO "default_ttl";
