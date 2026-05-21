-- Migrate existing task workspaces to the new tasks data model. This migration
-- identifies workspaces that were created as tasks (has_ai_task = true) and
-- populates the tasks and task_workspace_apps tables with their data.

-- Step 1: Create tasks from workspaces with has_ai_task TRUE in their latest build.
INSERT INTO tasks (
	id,
	organization_id,
	owner_id,
	name,
	workspace_id,
	template_version_id,
	template_parameters,
	prompt,
	created_at,
	deleted_at
)
SELECT
	gen_random_uuid() AS id,
	w.organization_id,
	w.owner_id,
	w.name,
	w.id AS workspace_id,
	latest_task_build.template_version_id,
	COALESCE(params.template_parameters, '{}'::jsonb) AS template_parameters,
	COALESCE(ai_prompt.value, '') AS prompt,
	w.created_at,
	CASE WHEN w.deleted = true THEN w.deleting_at ELSE NULL END AS deleted_at
FROM workspaces w
INNER JOIN LATERAL (
	-- Find the latest build for this workspace that has has_ai_task = true.
	SELECT
		wb.template_version_id
	FROM workspace_builds wb
	WHERE wb.workspace_id = w.id
		AND wb.has_ai_task = true
	ORDER BY wb.build_number DESC
	LIMIT 1
) latest_task_build ON true
LEFT JOIN LATERAL (
	-- Find the latest build that has a non-empty AI Prompt parameter.
	SELECT
		wb.id
	FROM workspace_builds wb
	WHERE wb.workspace_id = w.id
		AND EXISTS (
			SELECT 1
			FROM workspace_build_parameters wbp
			WHERE wbp.workspace_build_id = wb.id
				AND wbp.name = 'AI Prompt'
				AND wbp.value != ''
		)
	ORDER BY wb.build_number DESC
	LIMIT 1
) latest_prompt_build ON true
LEFT JOIN LATERAL (
	-- Extract the AI Prompt parameter value from the prompt build.
	SELECT wbp.value
	FROM workspace_build_parameters wbp
	WHERE wbp.workspace_build_id = latest_prompt_build.id
		AND wbp.name = 'AI Prompt'
	LIMIT 1
) ai_prompt ON true
LEFT JOIN LATERAL (
	-- Aggregate all other parameters (excluding AI Prompt) from the prompt build.
	SELECT jsonb_object_agg(wbp.name, wbp.value) AS template_parameters
	FROM workspace_build_parameters wbp
	WHERE wbp.workspace_build_id = latest_prompt_build.id
		AND wbp.name != 'AI Prompt'
) params ON true
WHERE
	-- Skip deleted workspaces because of duplicate name.
	w.deleted = false
	-- Safe-guard, do not create tasks for workspaces that are already tasks.
	AND NOT EXISTS (
		SELECT 1
		FROM tasks t
		WHERE t.workspace_id = w.id
	);

-- Step 2: Populate task_workspace_apps table with build/agent/app information.
INSERT INTO task_workspace_apps (
	task_id,
	workspace_build_number,
	workspace_agent_id,
	workspace_app_id
)
SELECT
	t.id AS task_id,
	latest_build.build_number AS workspace_build_number,
	sidebar_app.agent_id AS workspace_agent_id,
	sidebar_app.id AS workspace_app_id
FROM tasks t
INNER JOIN LATERAL (
	-- Find the latest build for this tasks workspace.
	SELECT
		wb.build_number,
		wb.ai_task_sidebar_app_id
	FROM workspace_builds wb
	WHERE wb.workspace_id = t.workspace_id
	ORDER BY wb.build_number DESC
	LIMIT 1
) latest_build ON true
-- Get the sidebar app (optional, can be NULL).
LEFT JOIN workspace_apps sidebar_app
	ON sidebar_app.id = latest_build.ai_task_sidebar_app_id
WHERE
	-- Safe-guard, do not create for existing tasks.
	NOT EXISTS (
		SELECT 1
		FROM task_workspace_apps twa
		WHERE twa.task_id = t.id
	);
