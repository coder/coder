-- Migrate existing task workspaces to the new tasks table
-- This migration populates the tasks and task_workspace_apps tables with data
-- from workspaces that have has_ai_task = true in their latest build.

-- Step 1: Create tasks from workspaces with has_ai_task = true
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
    gen_random_uuid() as id,
    w.organization_id,
    w.owner_id,
    w.name,
    w.id as workspace_id,
    latest_build.template_version_id,
    COALESCE(
        jsonb_object_agg(
            wbp.name,
            wbp.value
        ) FILTER (WHERE wbp.name != 'coder_ai_task_prompt'),
        '{}'::jsonb
    ) as template_parameters,
    COALESCE(
        MAX(wbp.value) FILTER (WHERE wbp.name = 'coder_ai_task_prompt'),
        ''
    ) as prompt,
    w.created_at,
    NULL as deleted_at
FROM workspaces w
INNER JOIN LATERAL (
    SELECT
        wb.id,
        wb.build_number,
        wb.template_version_id,
        wb.ai_task_sidebar_app_id
    FROM workspace_builds wb
    WHERE wb.workspace_id = w.id
      AND wb.has_ai_task = true
    ORDER BY wb.build_number DESC
    LIMIT 1
) latest_build ON true
LEFT JOIN workspace_build_parameters wbp
    ON wbp.workspace_build_id = latest_build.id
WHERE w.deleted = false
  -- Ensure we don't duplicate if migration is re-run
  AND NOT EXISTS (
    SELECT 1 FROM tasks t WHERE t.workspace_id = w.id
  )
GROUP BY
    w.id,
    w.organization_id,
    w.owner_id,
    w.name,
    w.created_at,
    latest_build.template_version_id;

-- Step 2: Populate task_workspace_apps table
INSERT INTO task_workspace_apps (
    task_id,
    workspace_build_number,
    workspace_agent_id,
    workspace_app_id
)
SELECT
    t.id as task_id,
    latest_build.build_number as workspace_build_number,
    COALESCE(
        sidebar_app_agent.id,
        first_agent.id
    ) as workspace_agent_id,
    COALESCE(
        latest_build.ai_task_sidebar_app_id,
        first_app.id
    ) as workspace_app_id
FROM tasks t
INNER JOIN LATERAL (
    SELECT
        wb.id as build_id,
        wb.build_number,
        wb.ai_task_sidebar_app_id,
        wb.job_id
    FROM workspace_builds wb
    WHERE wb.workspace_id = t.workspace_id
    ORDER BY wb.build_number DESC
    LIMIT 1
) latest_build ON true
-- Find the agent for the sidebar app if specified
LEFT JOIN workspace_apps sidebar_app
    ON sidebar_app.id = latest_build.ai_task_sidebar_app_id
LEFT JOIN workspace_agents sidebar_app_agent
    ON sidebar_app_agent.id = sidebar_app.agent_id
-- Find the first non-sub-agent as fallback
LEFT JOIN LATERAL (
    SELECT wa.id
    FROM workspace_agents wa
    INNER JOIN workspace_resources wr ON wr.id = wa.resource_id
    WHERE wr.job_id = latest_build.job_id
      AND wa.parent_id IS NULL
    ORDER BY wa.created_at
    LIMIT 1
) first_agent ON sidebar_app_agent.id IS NULL
-- Find the first app of the selected agent
LEFT JOIN LATERAL (
    SELECT wapp.id
    FROM workspace_apps wapp
    WHERE wapp.agent_id = COALESCE(sidebar_app_agent.id, first_agent.id)
    ORDER BY wapp.created_at
    LIMIT 1
) first_app ON latest_build.ai_task_sidebar_app_id IS NULL
-- Only insert where we have both agent and app
WHERE (sidebar_app_agent.id IS NOT NULL OR first_agent.id IS NOT NULL)
  AND (latest_build.ai_task_sidebar_app_id IS NOT NULL OR first_app.id IS NOT NULL)
  -- Prevent duplicates on re-run
  AND NOT EXISTS (
    SELECT 1
    FROM task_workspace_apps twa
    WHERE twa.task_id = t.id
  );
