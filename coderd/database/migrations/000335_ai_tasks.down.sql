DROP VIEW workspace_build_with_user;

DROP VIEW template_version_with_user;

DROP INDEX idx_template_versions_has_ai_task;

ALTER TABLE
    template_versions DROP COLUMN has_ai_task;

ALTER TABLE
    workspace_builds DROP CONSTRAINT workspace_builds_ai_tasks_sidebar_app_id_fkey;

ALTER TABLE
    workspace_builds DROP COLUMN ai_tasks_sidebar_app_id;

ALTER TABLE
    workspace_builds DROP COLUMN has_ai_task;

-- Recreate `workspace_build_with_user` as defined in dump.sql
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
    COALESCE(visible_users.avatar_url, '' :: text) AS initiator_by_avatar_url,
    COALESCE(visible_users.username, '' :: text) AS initiator_by_username,
    COALESCE(visible_users.name, '' :: text) AS initiator_by_name
FROM
    (
        workspace_builds
        LEFT JOIN visible_users ON (
            (workspace_builds.initiator_id = visible_users.id)
        )
    );

COMMENT ON VIEW workspace_build_with_user IS 'Joins in the username + avatar url of the initiated by user.';

-- Recreate `template_version_with_user` as defined in dump.sql
CREATE VIEW template_version_with_user AS
SELECT
    template_versions.id,
    template_versions.template_id,
    template_versions.organization_id,
    template_versions.created_at,
    template_versions.updated_at,
    template_versions.name,
    template_versions.readme,
    template_versions.job_id,
    template_versions.created_by,
    template_versions.external_auth_providers,
    template_versions.message,
    template_versions.archived,
    template_versions.source_example_id,
    COALESCE(visible_users.avatar_url, '' :: text) AS created_by_avatar_url,
    COALESCE(visible_users.username, '' :: text) AS created_by_username,
    COALESCE(visible_users.name, '' :: text) AS created_by_name
FROM
    (
        template_versions
        LEFT JOIN visible_users ON (
            (template_versions.created_by = visible_users.id)
        )
    );

COMMENT ON VIEW template_version_with_user IS 'Joins in the username + avatar url of the created by user.';
