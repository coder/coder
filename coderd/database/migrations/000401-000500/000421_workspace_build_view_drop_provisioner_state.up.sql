-- Drop and recreate workspace_build_with_user to exclude provisioner_state.
-- This avoids loading the large Terraform state blob (1-5 MB per workspace)
-- on every query that uses this view. The callers that need provisioner_state
-- now fetch it separately via GetWorkspaceBuildProvisionerStateByID.
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
    workspace_builds.job_id,
    workspace_builds.deadline,
    workspace_builds.reason,
    workspace_builds.daily_cost,
    workspace_builds.max_deadline,
    workspace_builds.template_version_preset_id,
    workspace_builds.has_ai_task,
    workspace_builds.has_external_agent,
    COALESCE(visible_users.avatar_url, ''::text) AS initiator_by_avatar_url,
    COALESCE(visible_users.username, ''::text) AS initiator_by_username,
    COALESCE(visible_users.name, ''::text) AS initiator_by_name
FROM
    workspace_builds
LEFT JOIN
    visible_users ON workspace_builds.initiator_id = visible_users.id;

COMMENT ON VIEW workspace_build_with_user IS 'Joins in the username + avatar url of the initiated by user.';
