CREATE TYPE workspace_app_health AS ENUM ('disabled', 'intializing', 'healthy', 'unhealthy');

ALTER TABLE ONLY workspace_apps
	ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT '-infinity',
	ADD COLUMN IF NOT EXISTS healthcheck_enabled boolean NOT NULL DEFAULT FALSE,
	ADD COLUMN IF NOT EXISTS healthcheck_period int NOT NULL DEFAULT 0,
	ADD COLUMN IF NOT EXISTS healthcheck_threshold int NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS health workspace_app_health NOT NULL DEFAULT 'disabled';
