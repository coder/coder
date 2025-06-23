-- Drop the check constraint first
ALTER TABLE workspace_builds DROP CONSTRAINT workspace_builds_ai_task_sidebar_app_id_required;

-- Revert ai_task_sidebar_app_id back to ai_tasks_sidebar_app_id in workspace_builds table
ALTER TABLE workspace_builds DROP CONSTRAINT workspace_builds_ai_task_sidebar_app_id_fkey;

ALTER TABLE workspace_builds RENAME COLUMN ai_task_sidebar_app_id TO ai_tasks_sidebar_app_id;

ALTER TABLE workspace_builds ADD CONSTRAINT workspace_builds_ai_tasks_sidebar_app_id_fkey FOREIGN KEY (ai_tasks_sidebar_app_id) REFERENCES workspace_apps(id);

-- Revert the workspace_build_with_user view to use the original column name
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
    workspace_builds.ai_tasks_sidebar_app_id,
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
