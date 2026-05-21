ALTER TABLE ONLY workspace_apps
	DROP COLUMN IF EXISTS healthcheck_url,
	DROP COLUMN IF EXISTS healthcheck_interval,
	DROP COLUMN IF EXISTS healthcheck_threshold,
	DROP COLUMN IF EXISTS health;

DROP TYPE workspace_app_health;
