ALTER TABLE workspace_apps
    DROP COLUMN IF EXISTS cors_behavior;

DROP TYPE IF EXISTS app_cors_behavior;