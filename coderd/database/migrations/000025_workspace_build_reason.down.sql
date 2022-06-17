ALTER TABLE ONLY workspace_builds
    DROP COLUMN IF EXISTS reason;

DROP TYPE build_reason;
