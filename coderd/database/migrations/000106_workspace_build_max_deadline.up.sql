ALTER TABLE "templates" ADD COLUMN "max_ttl" bigint DEFAULT '0'::bigint NOT NULL;

ALTER TABLE "workspace_builds" ADD COLUMN "max_deadline" timestamp with time zone DEFAULT '0001-01-01 00:00:00+00'::timestamp with time zone NOT NULL;
