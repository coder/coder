DROP VIEW template_version_with_user;

DROP VIEW workspace_build_with_user;

DROP VIEW template_with_names;

DROP VIEW workspaces_expanded;

DROP VIEW visible_users;

-- Recreate `visible_users` as described in dump.sql

CREATE VIEW visible_users AS
SELECT users.id, users.username, users.avatar_url
FROM users;

COMMENT ON VIEW visible_users IS 'Visible fields of users are allowed to be joined with other tables for including context of other resources.';

-- Recreate `workspace_build_with_user` as described in dump.sql

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
    COALESCE(
        visible_users.avatar_url,
        ''::text
    ) AS initiator_by_avatar_url,
    COALESCE(
        visible_users.username,
        ''::text
    ) AS initiator_by_username
FROM (
        workspace_builds
        LEFT JOIN visible_users ON (
            (
                workspace_builds.initiator_id = visible_users.id
            )
        )
    );

COMMENT ON VIEW workspace_build_with_user IS 'Joins in the username + avatar url of the initiated by user.';

-- Recreate `template_with_names` as described in dump.sql

CREATE VIEW template_with_names AS
SELECT
    templates.id,
    templates.created_at,
    templates.updated_at,
    templates.organization_id,
    templates.deleted,
    templates.name,
    templates.provisioner,
    templates.active_version_id,
    templates.description,
    templates.default_ttl,
    templates.created_by,
    templates.icon,
    templates.user_acl,
    templates.group_acl,
    templates.display_name,
    templates.allow_user_cancel_workspace_jobs,
    templates.allow_user_autostart,
    templates.allow_user_autostop,
    templates.failure_ttl,
    templates.time_til_dormant,
    templates.time_til_dormant_autodelete,
    templates.autostop_requirement_days_of_week,
    templates.autostop_requirement_weeks,
    templates.autostart_block_days_of_week,
    templates.require_active_version,
    templates.deprecated,
    templates.activity_bump,
    templates.max_port_sharing_level,
    templates.use_classic_parameter_flow,
    COALESCE(
        visible_users.avatar_url,
        ''::text
    ) AS created_by_avatar_url,
    COALESCE(
        visible_users.username,
        ''::text
    ) AS created_by_username,
    COALESCE(organizations.name, ''::text) AS organization_name,
    COALESCE(
        organizations.display_name,
        ''::text
    ) AS organization_display_name,
    COALESCE(organizations.icon, ''::text) AS organization_icon
FROM (
        (
            templates
            LEFT JOIN visible_users ON (
                (
                    templates.created_by = visible_users.id
                )
            )
        )
        LEFT JOIN organizations ON (
            (
                templates.organization_id = organizations.id
            )
        )
    );

COMMENT ON VIEW template_with_names IS 'Joins in the display name information such as username, avatar, and organization name.';

-- Recreate `template_version_with_user` as described in dump.sql

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
    COALESCE(
        visible_users.avatar_url,
        ''::text
    ) AS created_by_avatar_url,
    COALESCE(
        visible_users.username,
        ''::text
    ) AS created_by_username
FROM (
        template_versions
        LEFT JOIN visible_users ON (
            template_versions.created_by = visible_users.id
        )
    );

COMMENT ON VIEW template_version_with_user IS 'Joins in the username + avatar url of the created by user.';

-- Recreate `workspaces_expanded` as described in dump.sql

CREATE VIEW workspaces_expanded AS
SELECT
    workspaces.id,
    workspaces.created_at,
    workspaces.updated_at,
    workspaces.owner_id,
    workspaces.organization_id,
    workspaces.template_id,
    workspaces.deleted,
    workspaces.name,
    workspaces.autostart_schedule,
    workspaces.ttl,
    workspaces.last_used_at,
    workspaces.dormant_at,
    workspaces.deleting_at,
    workspaces.automatic_updates,
    workspaces.favorite,
    workspaces.next_start_at,
    visible_users.avatar_url AS owner_avatar_url,
    visible_users.username AS owner_username,
    organizations.name AS organization_name,
    organizations.display_name AS organization_display_name,
    organizations.icon AS organization_icon,
    organizations.description AS organization_description,
    templates.name AS template_name,
    templates.display_name AS template_display_name,
    templates.icon AS template_icon,
    templates.description AS template_description
FROM (
        (
            (
                workspaces
                JOIN visible_users ON (
                    (
                        workspaces.owner_id = visible_users.id
                    )
                )
            )
            JOIN organizations ON (
                (
                    workspaces.organization_id = organizations.id
                )
            )
        )
        JOIN templates ON (
            (
                workspaces.template_id = templates.id
            )
        )
    );

COMMENT ON VIEW workspaces_expanded IS 'Joins in the display name information such as username, avatar, and organization name.';
