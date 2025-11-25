-- Add display_name column to tasks table
ALTER TABLE tasks ADD COLUMN display_name VARCHAR(127) NOT NULL DEFAULT '';
COMMENT ON COLUMN tasks.display_name IS 'Display name is a custom, human-friendly task name.';

-- Backfill existing tasks with truncated prompt as display name
-- Replace newlines/tabs with spaces, truncate to 64 characters and add ellipsis if truncated
UPDATE tasks
SET display_name = CASE
	WHEN LENGTH(REGEXP_REPLACE(prompt, E'[\\n\\r\\t]+', ' ', 'g')) > 64
	THEN LEFT(REGEXP_REPLACE(prompt, E'[\\n\\r\\t]+', ' ', 'g'), 63) || '…'
	ELSE REGEXP_REPLACE(prompt, E'[\\n\\r\\t]+', ' ', 'g')
	END
WHERE display_name = '';

-- Recreate the tasks_with_status view to pick up the new display_name column.
-- PostgreSQL resolves the tasks.* wildcard when the view is created, not when
-- it's queried, so the view must be recreated after adding columns to tasks.
DROP VIEW IF EXISTS tasks_with_status;

CREATE VIEW
	tasks_with_status
AS
SELECT
	tasks.*,
	-- Combine component statuses with precedence: build -> agent -> app.
	CASE
		WHEN tasks.workspace_id IS NULL THEN 'pending'::task_status
		WHEN build_status.status != 'active' THEN build_status.status::task_status
		WHEN agent_status.status != 'active' THEN agent_status.status::task_status
		ELSE app_status.status::task_status
		END AS status,
	-- Attach debug information for troubleshooting status.
	jsonb_build_object(
		'build', jsonb_build_object(
		'transition', latest_build_raw.transition,
		'job_status', latest_build_raw.job_status,
		'computed', build_status.status
				 ),
		'agent', jsonb_build_object(
			'lifecycle_state', agent_raw.lifecycle_state,
			'computed', agent_status.status
				 ),
		'app', jsonb_build_object(
			'health', app_raw.health,
			'computed', app_status.status
			   )
	) AS status_debug,
	task_app.*,
	agent_raw.lifecycle_state AS workspace_agent_lifecycle_state,
	app_raw.health AS workspace_app_health,
	task_owner.*
FROM
	tasks
		CROSS JOIN LATERAL (
		SELECT
			vu.username AS owner_username,
			vu.name AS owner_name,
			vu.avatar_url AS owner_avatar_url
		FROM
			visible_users vu
		WHERE
			vu.id = tasks.owner_id
			) task_owner
		LEFT JOIN LATERAL (
		SELECT
			task_app.workspace_build_number,
			task_app.workspace_agent_id,
			task_app.workspace_app_id
		FROM
			task_workspace_apps task_app
		WHERE
			task_id = tasks.id
		ORDER BY
			task_app.workspace_build_number DESC
			LIMIT 1
	) task_app ON TRUE

	-- Join the raw data for computing task status.
	LEFT JOIN LATERAL (
		SELECT
			workspace_build.transition,
			provisioner_job.job_status,
			workspace_build.job_id
		FROM
			workspace_builds workspace_build
		JOIN
			provisioner_jobs provisioner_job
			ON provisioner_job.id = workspace_build.job_id
		WHERE
			workspace_build.workspace_id = tasks.workspace_id
			AND workspace_build.build_number = task_app.workspace_build_number
	) latest_build_raw ON TRUE
	LEFT JOIN LATERAL (
		SELECT
			workspace_agent.lifecycle_state
		FROM
			workspace_agents workspace_agent
		WHERE
			workspace_agent.id = task_app.workspace_agent_id
	) agent_raw ON TRUE
	LEFT JOIN LATERAL (
		SELECT
			workspace_app.health
		FROM
			workspace_apps workspace_app
		WHERE
			workspace_app.id = task_app.workspace_app_id
	) app_raw ON TRUE

	-- Compute the status for each component.
	CROSS JOIN LATERAL (
		SELECT
			CASE
				WHEN latest_build_raw.job_status IS NULL THEN 'pending'::task_status
				WHEN latest_build_raw.job_status IN ('failed', 'canceling', 'canceled') THEN 'error'::task_status
				WHEN
					latest_build_raw.transition IN ('stop', 'delete')
					AND latest_build_raw.job_status = 'succeeded' THEN 'paused'::task_status
				WHEN
					latest_build_raw.transition = 'start'
					AND latest_build_raw.job_status = 'pending' THEN 'initializing'::task_status
				-- Build is running or done, defer to agent/app status.
				WHEN
					latest_build_raw.transition = 'start'
					AND latest_build_raw.job_status IN ('running', 'succeeded') THEN 'active'::task_status
				ELSE 'unknown'::task_status
			END AS status
	) build_status
	CROSS JOIN LATERAL (
		SELECT
			CASE
				-- No agent or connecting.
				WHEN
					agent_raw.lifecycle_state IS NULL
					OR agent_raw.lifecycle_state IN ('created', 'starting') THEN 'initializing'::task_status
				-- Agent is running, defer to app status.
				-- NOTE(mafredri): The start_error/start_timeout states means connected, but some startup script failed.
				-- This may or may not affect the task status but this has to be caught by app health check.
				WHEN agent_raw.lifecycle_state IN ('ready', 'start_timeout', 'start_error') THEN 'active'::task_status
				-- If the agent is shutting down or turned off, this is an unknown state because we would expect a stop
				-- build to be running.
				-- This is essentially equal to: `IN ('shutting_down', 'shutdown_timeout', 'shutdown_error', 'off')`,
				-- but we cannot use them because the values were added in a migration.
				WHEN agent_raw.lifecycle_state NOT IN ('created', 'starting', 'ready', 'start_timeout', 'start_error') THEN 'unknown'::task_status
				ELSE 'unknown'::task_status
			END AS status
	) agent_status
	CROSS JOIN LATERAL (
		SELECT
			CASE
				WHEN app_raw.health = 'initializing' THEN 'initializing'::task_status
				WHEN app_raw.health = 'unhealthy' THEN 'error'::task_status
				WHEN app_raw.health IN ('healthy', 'disabled') THEN 'active'::task_status
				ELSE 'unknown'::task_status
			END AS status
	) app_status
	WHERE
		tasks.deleted_at IS NULL;
