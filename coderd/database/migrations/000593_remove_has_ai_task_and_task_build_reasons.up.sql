DROP VIEW IF EXISTS template_version_with_user;
DROP VIEW IF EXISTS workspace_build_with_user;

-- Drop has_ai_task index and columns.
DROP INDEX IF EXISTS idx_template_versions_has_ai_task;
ALTER TABLE template_versions DROP COLUMN IF EXISTS has_ai_task;
ALTER TABLE workspace_builds DROP COLUMN IF EXISTS has_ai_task;

-- Remove task-related build reasons from build_reason enum. Automatic
-- pauses stay system-attributed; manual pauses and resumes were created by
-- authenticated user requests, so they keep user attribution as initiator.
UPDATE workspace_builds
SET reason = 'autostop'
WHERE reason::text = 'task_auto_pause';

UPDATE workspace_builds
SET reason = 'initiator'
WHERE reason::text IN ('task_manual_pause', 'task_resume');

UPDATE workspace_build_orchestrations
SET child_reason = 'autostop'
WHERE child_reason::text = 'task_auto_pause';

UPDATE workspace_build_orchestrations
SET child_reason = 'initiator'
WHERE child_reason::text IN ('task_manual_pause', 'task_resume');

ALTER TYPE build_reason RENAME TO build_reason_old;

CREATE TYPE build_reason AS ENUM (
    'initiator',
    'autostart',
    'autostop',
    'dormancy',
    'failedstop',
    'autodelete',
    'dashboard',
    'cli',
    'ssh_connection',
    'vscode_connection',
    'jetbrains_connection'
);

ALTER TABLE workspace_builds ALTER COLUMN reason DROP DEFAULT;
ALTER TABLE workspace_builds ALTER COLUMN reason TYPE build_reason USING (reason::text::build_reason);
ALTER TABLE workspace_builds ALTER COLUMN reason SET DEFAULT 'initiator'::build_reason;

ALTER TABLE workspace_build_orchestrations ALTER COLUMN child_reason TYPE build_reason USING (child_reason::text::build_reason);

DROP TYPE build_reason_old;

-- Recreate views without task columns.
CREATE VIEW template_version_with_user AS
 SELECT template_versions.id,
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
    template_versions.has_external_agent,
    COALESCE(visible_users.avatar_url, ''::text) AS created_by_avatar_url,
    COALESCE(visible_users.username, ''::text) AS created_by_username,
    COALESCE(visible_users.name, ''::text) AS created_by_name
   FROM (template_versions
     LEFT JOIN visible_users ON ((template_versions.created_by = visible_users.id)));

COMMENT ON VIEW template_version_with_user IS 'Joins in the username + avatar url of the created by user.';

CREATE VIEW workspace_build_with_user AS
 SELECT workspace_builds.id,
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
    workspace_builds.has_external_agent,
    workspace_builds.notified_autostop_deadline,
    COALESCE(visible_users.avatar_url, ''::text) AS initiator_by_avatar_url,
    COALESCE(visible_users.username, ''::text) AS initiator_by_username,
    COALESCE(visible_users.name, ''::text) AS initiator_by_name
   FROM (workspace_builds
     LEFT JOIN visible_users ON ((workspace_builds.initiator_id = visible_users.id)));

COMMENT ON VIEW workspace_build_with_user IS 'Joins in the username + avatar url of the initiated by user.';
