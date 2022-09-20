CREATE TYPE workspace_app_health AS ENUM ('disabled', 'initializing', 'healthy', 'unhealthy');

ALTER TABLE ONLY workspace_apps
	ADD COLUMN IF NOT EXISTS healthcheck_enabled boolean NOT NULL DEFAULT FALSE,
	ADD COLUMN IF NOT EXISTS healthcheck_url text NOT NULL DEFAULT '',
	ADD COLUMN IF NOT EXISTS healthcheck_interval int NOT NULL DEFAULT 0,
	ADD COLUMN IF NOT EXISTS healthcheck_threshold int NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS health workspace_app_health NOT NULL DEFAULT 'disabled';
