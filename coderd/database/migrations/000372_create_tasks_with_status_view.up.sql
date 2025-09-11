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
					WHEN agents_status.none THEN 'initializing'::task_status
					WHEN agents_status.connecting THEN 'initializing'::task_status
					WHEN agents_status.connected THEN
						CASE
							WHEN apps_status.any_unhealthy THEN 'error'::task_status
							WHEN apps_status.any_initializing THEN 'initializing'::task_status
							WHEN apps_status.all_healthy_or_disabled THEN 'active'::task_status
							ELSE 'unknown'::task_status
						END
					ELSE 'unknown'::task_status
				END

			ELSE 'unknown'::task_status
		END AS status
	FROM
		tasks
	LEFT JOIN (
		SELECT DISTINCT ON (workspace_build.workspace_id) workspace_build.workspace_id,
			workspace_build.transition,
			provisioner_job.job_status
		FROM workspace_builds workspace_build
		JOIN provisioner_jobs provisioner_job ON provisioner_job.id = workspace_build.job_id
		ORDER BY workspace_build.workspace_id, workspace_build.build_number DESC
	) latest_build ON latest_build.workspace_id = tasks.workspace_id
	CROSS JOIN LATERAL (
		SELECT
			COUNT(*) = 0 AS none,
			bool_or(workspace_agent.lifecycle_state IN ('created', 'starting')) AS connecting,
			bool_and(workspace_agent.lifecycle_state = 'ready') AS connected
		FROM task_workspace_apps task_app
		JOIN workspace_agents workspace_agent ON workspace_agent.id = task_app.workspace_agent_id
		WHERE task_app.task_id = tasks.id
			AND workspace_agent.deleted = false
	) agents_status
	CROSS JOIN LATERAL (
		SELECT
			bool_or(workspace_app.health = 'unhealthy') AS any_unhealthy,
			bool_or(workspace_app.health = 'initializing') AS any_initializing,
			bool_and(workspace_app.health IN ('healthy', 'disabled')) AS all_healthy_or_disabled
		FROM task_workspace_apps task_app
		JOIN workspace_apps workspace_app ON workspace_app.id = task_app.workspace_app_id
		WHERE task_app.task_id = tasks.id
	) apps_status
	WHERE
		tasks.deleted_at IS NULL;
