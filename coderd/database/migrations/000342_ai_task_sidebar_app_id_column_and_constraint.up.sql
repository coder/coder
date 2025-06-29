-- Rename ai_tasks_sidebar_app_id to ai_task_sidebar_app_id in workspace_builds table
ALTER TABLE workspace_builds DROP CONSTRAINT workspace_builds_ai_tasks_sidebar_app_id_fkey;

ALTER TABLE workspace_builds RENAME COLUMN ai_tasks_sidebar_app_id TO ai_task_sidebar_app_id;

ALTER TABLE workspace_builds ADD CONSTRAINT workspace_builds_ai_task_sidebar_app_id_fkey FOREIGN KEY (ai_task_sidebar_app_id) REFERENCES workspace_apps(id);

-- if has_ai_task is true, ai_task_sidebar_app_id MUST be set
-- ai_task_sidebar_app_id can ONLY be set if has_ai_task is true
--
--   has_ai_task | ai_task_sidebar_app_id | Result
--   ------------|------------------------|---------------
--   NULL        | NULL                   | TRUE (passes)
--   NULL        | NOT NULL               | FALSE (fails)
--   FALSE       | NULL                   | TRUE (passes)
--   FALSE       | NOT NULL               | FALSE (fails)
--   TRUE        | NULL                   | FALSE (fails)
--   TRUE        | NOT NULL               | TRUE (passes)
ALTER TABLE workspace_builds
	ADD CONSTRAINT workspace_builds_ai_task_sidebar_app_id_required CHECK (
		((has_ai_task IS NULL OR has_ai_task = false) AND ai_task_sidebar_app_id IS NULL)
			OR (has_ai_task = true AND ai_task_sidebar_app_id IS NOT NULL)
		);

-- Update the workspace_build_with_user view to use the new column name
DROP VIEW workspace_build_with_user;

CREATE VIEW workspace_build_with_user AS
SELECT
    workspace_builds.id,
    workspace_builds.created_at,
    workspace_builds.updated_at,
    workspace_builds.workspace_id,
    workspace_builds.template_version_id,
    workspace_builds.build_number,
    workspace_builds.transition,
    workspace_builds.initiator_id,
    workspace_builds.provisioner_state,
    workspace_builds.job_id,
    workspace_builds.deadline,
    workspace_builds.reason,
    workspace_builds.daily_cost,
    workspace_builds.max_deadline,
    workspace_builds.template_version_preset_id,
    workspace_builds.has_ai_task,
    workspace_builds.ai_task_sidebar_app_id,
    COALESCE(
        visible_users.avatar_url,
        '' :: text
    ) AS initiator_by_avatar_url,
    COALESCE(
        visible_users.username,
        '' :: text
    ) AS initiator_by_username,
    COALESCE(visible_users.name, '' :: text) AS initiator_by_name
FROM
    (
        workspace_builds
        LEFT JOIN visible_users ON (
            (
                workspace_builds.initiator_id = visible_users.id
            )
        )
    );

COMMENT ON VIEW workspace_build_with_user IS 'Joins in the username + avatar url of the initiated by user.';
