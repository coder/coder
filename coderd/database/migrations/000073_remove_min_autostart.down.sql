-- add "slug" min_autostart_interval to "templates" table
ALTER TABLE "templates" ADD COLUMN "min_autostart_interval" int DEFAULT 0;

-- rename "default_ttl" to "max_ttl" on "templates" table
ALTER TABLE "templates" RENAME COLUMN "default_ttl" TO "max_ttl";
