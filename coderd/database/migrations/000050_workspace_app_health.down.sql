ALTER TABLE ONLY workspace_apps
    DROP COLUMN IF EXISTS health;

DROP TYPE workspace_app_health;
