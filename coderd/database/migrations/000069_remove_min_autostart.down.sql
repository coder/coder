-- add "slug" min_autostart_interval to "templates" table
ALTER TABLE "templates" ADD COLUMN "min_autostart_interval" int DEFAULT 0;
