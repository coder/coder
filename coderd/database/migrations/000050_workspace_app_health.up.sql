CREATE TYPE workspace_app_health AS ENUM ('intializing', 'healthy', 'unhealthy');

ALTER TABLE ONLY workspace_apps
    ADD COLUMN IF NOT EXISTS health workspace_app_health NOT NULL DEFAULT 'intializing',
	ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT '-infinity';
