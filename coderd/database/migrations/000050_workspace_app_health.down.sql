ALTER TABLE ONLY workspace_apps
	DROP COLUMN IF EXISTS updated_at,
	DROP COLUMN IF EXISTS healthcheck_enabled,
	DROP COLUMN IF EXISTS healthcheck_period,
	DROP COLUMN IF EXISTS unhealthy_threshold,
	DROP COLUMN IF EXISTS health;

DROP TYPE workspace_app_health;
