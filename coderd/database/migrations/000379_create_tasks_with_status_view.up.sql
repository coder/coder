-- Replace workspace_build_id with workspace_build_number.
ALTER TABLE task_workspace_apps
	ADD COLUMN workspace_build_number INTEGER;

-- Try to populate workspace_build_number from workspace_builds.
UPDATE task_workspace_apps
SET workspace_build_number = workspace_builds.build_number
FROM workspace_builds
WHERE workspace_builds.id = task_workspace_apps.workspace_build_id;

-- Remove rows that couldn't be migrated.
DELETE FROM task_workspace_apps
WHERE workspace_build_number IS NULL;

ALTER TABLE task_workspace_apps
	DROP COLUMN workspace_build_id,
	ALTER COLUMN workspace_build_number SET NOT NULL,
	ALTER COLUMN workspace_agent_id DROP NOT NULL,
	ALTER COLUMN workspace_app_id DROP NOT NULL,
	ADD CONSTRAINT task_workspace_apps_pkey PRIMARY KEY (task_id, workspace_build_number);

-- Add indexes for common joins or filters.
CREATE INDEX IF NOT EXISTS tasks_workspace_id_idx ON tasks (workspace_id);
CREATE INDEX IF NOT EXISTS tasks_owner_id_idx ON tasks (owner_id);
CREATE INDEX IF NOT EXISTS tasks_organization_id_idx ON tasks (organization_id);

CREATE TYPE task_status AS ENUM (
	'pending',
	'initializing',
	'active',
	'paused',
	'unknown',
	'error'
);

CREATE VIEW
	tasks_with_status
AS
	SELECT
		tasks.*,
		CASE
			WHEN tasks.workspace_id IS NULL OR latest_build.job_status IS NULL THEN 'pending'::task_status

			WHEN latest_build.job_status = 'failed' THEN 'error'::task_status

			WHEN latest_build.transition IN ('stop', 'delete')
				AND latest_build.job_status = 'succeeded' THEN 'paused'::task_status

			WHEN latest_build.transition = 'start'
				AND latest_build.job_status = 'pending' THEN 'initializing'::task_status

			WHEN latest_build.transition = 'start' AND latest_build.job_status IN ('running', 'succeeded') THEN
				CASE
					WHEN agent_status.none THEN 'initializing'::task_status
					WHEN agent_status.connecting THEN 'initializing'::task_status
					WHEN agent_status.connected THEN
						CASE
							WHEN app_status.any_unhealthy THEN 'error'::task_status
							WHEN app_status.any_initializing THEN 'initializing'::task_status
							WHEN app_status.all_healthy_or_disabled THEN 'active'::task_status
							ELSE 'unknown'::task_status
						END
					ELSE 'unknown'::task_status
				END

			ELSE 'unknown'::task_status
		END AS status
	FROM
		tasks
	LEFT JOIN LATERAL (
		SELECT workspace_build_number, workspace_agent_id, workspace_app_id
		FROM task_workspace_apps task_app
		WHERE task_id = tasks.id
		ORDER BY workspace_build_number DESC
		LIMIT 1
	) task_app ON TRUE
	LEFT JOIN LATERAL (
		SELECT
			workspace_build.transition,
			provisioner_job.job_status,
			workspace_build.job_id
		FROM workspace_builds workspace_build
		JOIN provisioner_jobs provisioner_job ON provisioner_job.id = workspace_build.job_id
		WHERE workspace_build.workspace_id = tasks.workspace_id
			AND workspace_build.build_number = task_app.workspace_build_number
	) latest_build ON TRUE
	CROSS JOIN LATERAL (
		SELECT
			COUNT(*) = 0 AS none,
			bool_or(workspace_agent.lifecycle_state IN ('created', 'starting')) AS connecting,
			bool_and(workspace_agent.lifecycle_state = 'ready') AS connected
		FROM workspace_agents workspace_agent
		WHERE workspace_agent.id = task_app.workspace_agent_id
	) agent_status
	CROSS JOIN LATERAL (
		SELECT
			bool_or(workspace_app.health = 'unhealthy') AS any_unhealthy,
			bool_or(workspace_app.health = 'initializing') AS any_initializing,
			bool_and(workspace_app.health IN ('healthy', 'disabled')) AS all_healthy_or_disabled
		FROM workspace_apps workspace_app
		WHERE workspace_app.id = task_app.workspace_app_id
	) app_status
	WHERE
		tasks.deleted_at IS NULL;
