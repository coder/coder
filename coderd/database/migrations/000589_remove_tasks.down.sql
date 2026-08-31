-- Drop views before modifying underlying tables and columns.
DROP VIEW IF EXISTS workspaces_expanded;
DROP VIEW IF EXISTS template_version_with_user;
DROP VIEW IF EXISTS workspace_build_with_user;

-- Recreate task status enum type.
CREATE TYPE task_status AS ENUM (
    'pending',
    'initializing',
    'active',
    'paused',
    'unknown',
    'error'
);

-- Recreate tasks table.
CREATE TABLE tasks (
    id uuid NOT NULL,
    organization_id uuid NOT NULL,
    owner_id uuid NOT NULL,
    name text NOT NULL,
    workspace_id uuid,
    template_version_id uuid NOT NULL,
    template_parameters jsonb DEFAULT '{}'::jsonb NOT NULL,
    prompt text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    deleted_at timestamp with time zone,
    display_name character varying(127) DEFAULT ''::character varying NOT NULL
);

COMMENT ON COLUMN tasks.display_name IS 'Display name is a custom, human-friendly task name.';

ALTER TABLE ONLY tasks
    ADD CONSTRAINT tasks_pkey PRIMARY KEY (id);

CREATE INDEX tasks_organization_id_idx ON tasks USING btree (organization_id);
CREATE INDEX tasks_owner_id_idx ON tasks USING btree (owner_id);
CREATE UNIQUE INDEX tasks_owner_id_name_unique_idx ON tasks USING btree (owner_id, lower(name)) WHERE (deleted_at IS NULL);
COMMENT ON INDEX tasks_owner_id_name_unique_idx IS 'Index to ensure uniqueness for task owner/name';
CREATE INDEX tasks_workspace_id_idx ON tasks USING btree (workspace_id);

ALTER TABLE ONLY tasks
    ADD CONSTRAINT tasks_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE ONLY tasks
    ADD CONSTRAINT tasks_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE ONLY tasks
    ADD CONSTRAINT tasks_template_version_id_fkey FOREIGN KEY (template_version_id) REFERENCES template_versions(id) ON DELETE CASCADE;
ALTER TABLE ONLY tasks
    ADD CONSTRAINT tasks_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE;

-- Recreate task_workspace_apps table.
CREATE TABLE task_workspace_apps (
    task_id uuid NOT NULL,
    workspace_agent_id uuid,
    workspace_app_id uuid,
    workspace_build_number integer NOT NULL
);

ALTER TABLE ONLY task_workspace_apps
    ADD CONSTRAINT task_workspace_apps_pkey PRIMARY KEY (task_id, workspace_build_number);

ALTER TABLE ONLY task_workspace_apps
    ADD CONSTRAINT task_workspace_apps_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;
ALTER TABLE ONLY task_workspace_apps
    ADD CONSTRAINT task_workspace_apps_workspace_agent_id_fkey FOREIGN KEY (workspace_agent_id) REFERENCES workspace_agents(id) ON DELETE CASCADE;
ALTER TABLE ONLY task_workspace_apps
    ADD CONSTRAINT task_workspace_apps_workspace_app_id_fkey FOREIGN KEY (workspace_app_id) REFERENCES workspace_apps(id) ON DELETE CASCADE;

-- Recreate task_snapshots table.
CREATE TABLE task_snapshots (
    task_id uuid NOT NULL,
    log_snapshot jsonb NOT NULL,
    log_snapshot_created_at timestamp with time zone DEFAULT now() NOT NULL
);

COMMENT ON TABLE task_snapshots IS 'Stores snapshots of task state when paused, currently limited to conversation history.';
COMMENT ON COLUMN task_snapshots.task_id IS 'The task this snapshot belongs to.';
COMMENT ON COLUMN task_snapshots.log_snapshot IS 'Task conversation history in JSON format, allowing users to view logs when the workspace is stopped.';
COMMENT ON COLUMN task_snapshots.log_snapshot_created_at IS 'When this log snapshot was captured.';

ALTER TABLE ONLY task_snapshots
    ADD CONSTRAINT task_snapshots_pkey PRIMARY KEY (task_id);

ALTER TABLE ONLY task_snapshots
    ADD CONSTRAINT task_snapshots_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;

-- Recreate has_ai_task columns and index. The dropped values are not
-- recoverable, so backfill false to satisfy older migrations that restore
-- NOT NULL on the way further down.
ALTER TABLE template_versions ADD COLUMN has_ai_task boolean DEFAULT false;
ALTER TABLE template_versions ALTER COLUMN has_ai_task DROP DEFAULT;
CREATE INDEX idx_template_versions_has_ai_task ON template_versions USING btree (has_ai_task);
ALTER TABLE workspace_builds ADD COLUMN has_ai_task boolean DEFAULT false;
ALTER TABLE workspace_builds ALTER COLUMN has_ai_task DROP DEFAULT;

-- Recreate build_reason enum with task values.
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
    'jetbrains_connection',
    'task_auto_pause',
    'task_manual_pause',
    'task_resume'
);

ALTER TABLE workspace_builds ALTER COLUMN reason DROP DEFAULT;
ALTER TABLE workspace_builds ALTER COLUMN reason TYPE build_reason USING (reason::text::build_reason);
ALTER TABLE workspace_builds ALTER COLUMN reason SET DEFAULT 'initiator'::build_reason;

ALTER TABLE workspace_build_orchestrations ALTER COLUMN child_reason TYPE build_reason USING (child_reason::text::build_reason);

DROP TYPE build_reason_old;

-- Recreate views.
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
    template_versions.has_ai_task,
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
    workspace_builds.has_ai_task,
    workspace_builds.has_external_agent,
    workspace_builds.notified_autostop_deadline,
    COALESCE(visible_users.avatar_url, ''::text) AS initiator_by_avatar_url,
    COALESCE(visible_users.username, ''::text) AS initiator_by_username,
    COALESCE(visible_users.name, ''::text) AS initiator_by_name
   FROM (workspace_builds
     LEFT JOIN visible_users ON ((workspace_builds.initiator_id = visible_users.id)));

COMMENT ON VIEW workspace_build_with_user IS 'Joins in the username + avatar url of the initiated by user.';

CREATE VIEW workspaces_expanded AS
 SELECT workspaces.id,
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
    workspaces.group_acl,
    workspaces.user_acl,
    visible_users.avatar_url AS owner_avatar_url,
    visible_users.username AS owner_username,
    visible_users.name AS owner_name,
    organizations.name AS organization_name,
    organizations.display_name AS organization_display_name,
    organizations.icon AS organization_icon,
    organizations.description AS organization_description,
    templates.name AS template_name,
    templates.display_name AS template_display_name,
    templates.icon AS template_icon,
    templates.description AS template_description,
    tasks.id AS task_id,
    COALESCE(( SELECT jsonb_object_agg(acl.key, jsonb_build_object('name', COALESCE(g.name, ''::text), 'avatar_url', COALESCE(g.avatar_url, ''::text))) AS jsonb_object_agg
           FROM (jsonb_each(workspaces.group_acl) acl(key, value)
             LEFT JOIN groups g ON ((g.id = (acl.key)::uuid)))), '{}'::jsonb) AS group_acl_display_info,
    COALESCE(( SELECT jsonb_object_agg(acl.key, jsonb_build_object('name', COALESCE(vu.name, ''::text), 'avatar_url', COALESCE(vu.avatar_url, ''::text))) AS jsonb_object_agg
           FROM (jsonb_each(workspaces.user_acl) acl(key, value)
             LEFT JOIN visible_users vu ON ((vu.id = (acl.key)::uuid)))), '{}'::jsonb) AS user_acl_display_info
   FROM ((((workspaces
     JOIN visible_users ON ((workspaces.owner_id = visible_users.id)))
     JOIN organizations ON ((workspaces.organization_id = organizations.id)))
     JOIN templates ON ((workspaces.template_id = templates.id)))
     LEFT JOIN tasks ON ((workspaces.id = tasks.workspace_id)));

COMMENT ON VIEW workspaces_expanded IS 'Joins in the display name information such as username, avatar, and organization name.';

CREATE VIEW tasks_with_status AS
 SELECT tasks.id,
    tasks.organization_id,
    tasks.owner_id,
    tasks.name,
    tasks.workspace_id,
    tasks.template_version_id,
    tasks.template_parameters,
    tasks.prompt,
    tasks.created_at,
    tasks.deleted_at,
    tasks.display_name,
    COALESCE(workspaces.group_acl, '{}'::jsonb) AS workspace_group_acl,
    COALESCE(workspaces.user_acl, '{}'::jsonb) AS workspace_user_acl,
        CASE
            WHEN (tasks.workspace_id IS NULL) THEN 'pending'::task_status
            WHEN (build_status.status <> 'active'::task_status) THEN build_status.status
            WHEN (agent_status.status <> 'active'::task_status) THEN agent_status.status
            ELSE app_status.status
        END AS status,
    jsonb_build_object('build', jsonb_build_object('transition', latest_build_raw.transition, 'job_status', latest_build_raw.job_status, 'computed', build_status.status), 'agent', jsonb_build_object('lifecycle_state', agent_raw.lifecycle_state, 'computed', agent_status.status), 'app', jsonb_build_object('health', app_raw.health, 'computed', app_status.status)) AS status_debug,
    task_app.workspace_build_number,
    task_app.workspace_agent_id,
    task_app.workspace_app_id,
    agent_raw.lifecycle_state AS workspace_agent_lifecycle_state,
    app_raw.health AS workspace_app_health,
    task_owner.owner_username,
    task_owner.owner_name,
    task_owner.owner_avatar_url
   FROM (((((((((tasks
     LEFT JOIN workspaces ON ((workspaces.id = tasks.workspace_id)))
     CROSS JOIN LATERAL ( SELECT vu.username AS owner_username,
            vu.name AS owner_name,
            vu.avatar_url AS owner_avatar_url
           FROM visible_users vu
          WHERE (vu.id = tasks.owner_id)) task_owner)
     LEFT JOIN LATERAL ( SELECT task_app_1.workspace_build_number,
            task_app_1.workspace_agent_id,
            task_app_1.workspace_app_id
           FROM task_workspace_apps task_app_1
          WHERE (task_app_1.task_id = tasks.id)
          ORDER BY task_app_1.workspace_build_number DESC
         LIMIT 1) task_app ON (true))
     LEFT JOIN LATERAL ( SELECT workspace_build.transition,
            provisioner_job.job_status,
            workspace_build.job_id
           FROM (workspace_builds workspace_build
             JOIN provisioner_jobs provisioner_job ON ((provisioner_job.id = workspace_build.job_id)))
          WHERE ((workspace_build.workspace_id = tasks.workspace_id) AND (workspace_build.build_number = task_app.workspace_build_number))) latest_build_raw ON (true))
     LEFT JOIN LATERAL ( SELECT workspace_agent.lifecycle_state
           FROM workspace_agents workspace_agent
          WHERE (workspace_agent.id = task_app.workspace_agent_id)) agent_raw ON (true))
     LEFT JOIN LATERAL ( SELECT workspace_app.health
           FROM workspace_apps workspace_app
          WHERE (workspace_app.id = task_app.workspace_app_id)) app_raw ON (true))
     CROSS JOIN LATERAL ( SELECT
                CASE
                    WHEN (latest_build_raw.job_status IS NULL) THEN 'pending'::task_status
                    WHEN (latest_build_raw.job_status = ANY (ARRAY['failed'::provisioner_job_status, 'canceling'::provisioner_job_status, 'canceled'::provisioner_job_status])) THEN 'error'::task_status
                    WHEN ((latest_build_raw.transition = ANY (ARRAY['stop'::workspace_transition, 'delete'::workspace_transition])) AND (latest_build_raw.job_status = 'succeeded'::provisioner_job_status)) THEN 'paused'::task_status
                    WHEN ((latest_build_raw.transition = 'start'::workspace_transition) AND (latest_build_raw.job_status = 'pending'::provisioner_job_status)) THEN 'pending'::task_status
                    WHEN ((latest_build_raw.transition = 'start'::workspace_transition) AND (latest_build_raw.job_status = ANY (ARRAY['running'::provisioner_job_status, 'succeeded'::provisioner_job_status]))) THEN 'active'::task_status
                    ELSE 'unknown'::task_status
                END AS status) build_status)
     CROSS JOIN LATERAL ( SELECT
                CASE
                    WHEN ((agent_raw.lifecycle_state IS NULL) OR (agent_raw.lifecycle_state = ANY (ARRAY['created'::workspace_agent_lifecycle_state, 'starting'::workspace_agent_lifecycle_state]))) THEN 'initializing'::task_status
                    WHEN (agent_raw.lifecycle_state = ANY (ARRAY['ready'::workspace_agent_lifecycle_state, 'start_timeout'::workspace_agent_lifecycle_state, 'start_error'::workspace_agent_lifecycle_state])) THEN 'active'::task_status
                    WHEN (agent_raw.lifecycle_state <> ALL (ARRAY['created'::workspace_agent_lifecycle_state, 'starting'::workspace_agent_lifecycle_state, 'ready'::workspace_agent_lifecycle_state, 'start_timeout'::workspace_agent_lifecycle_state, 'start_error'::workspace_agent_lifecycle_state])) THEN 'unknown'::task_status
                    ELSE 'unknown'::task_status
                END AS status) agent_status)
     CROSS JOIN LATERAL ( SELECT
                CASE
                    WHEN (app_raw.health = 'initializing'::workspace_app_health) THEN 'initializing'::task_status
                    WHEN (app_raw.health = 'unhealthy'::workspace_app_health) THEN 'error'::task_status
                    WHEN (app_raw.health = ANY (ARRAY['healthy'::workspace_app_health, 'disabled'::workspace_app_health])) THEN 'active'::task_status
                    ELSE 'unknown'::task_status
                END AS status) app_status)
  WHERE (tasks.deleted_at IS NULL);

-- Restore task notification templates. Inbox notification history deleted
-- by the cascade is not recoverable.
INSERT INTO notification_templates (
    id, name, title_template, body_template, actions, "group", method, kind, enabled_by_default
) VALUES (
    'bd4b7168-d05e-4e19-ad0f-3593b77aa90f',
    'Task Working',
    E'Task ''{{.Labels.workspace}}'' is working',
    E'The task ''{{.Labels.task}}'' transitioned to a working state.',
    '[{"label": "View task", "url": "{{base_url}}/tasks/{{.UserUsername}}/{{.Labels.workspace}}"}, {"label": "View workspace", "url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"}]'::jsonb,
    'Task Events',
    NULL,
    'system'::notification_template_kind,
    false
), (
    'd4a6271c-cced-4ed0-84ad-afd02a9c7799',
    'Task Idle',
    E'Task ''{{.Labels.workspace}}'' is idle',
    E'The task ''{{.Labels.task}}'' is idle and ready for input.',
    '[{"label": "View task", "url": "{{base_url}}/tasks/{{.UserUsername}}/{{.Labels.workspace}}"}, {"label": "View workspace", "url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"}]'::jsonb,
    'Task Events',
    NULL,
    'system'::notification_template_kind,
    false
), (
    '8c5a4d12-9f7e-4b3a-a1c8-6e4f2d9b5a7c',
    'Task Completed',
    E'Task ''{{.Labels.workspace}}'' completed',
    E'The task ''{{.Labels.task}}'' has completed successfully.',
    '[{"label": "View task", "url": "{{base_url}}/tasks/{{.UserUsername}}/{{.Labels.workspace}}"}, {"label": "View workspace", "url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"}]'::jsonb,
    'Task Events',
    NULL,
    'system'::notification_template_kind,
    false
), (
    '3b7e8f1a-4c2d-49a6-b5e9-7f3a1c8d6b4e',
    'Task Failed',
    E'Task ''{{.Labels.workspace}}'' failed',
    E'The task ''{{.Labels.task}}'' has failed. Check the logs for more details.',
    '[{"label": "View task", "url": "{{base_url}}/tasks/{{.UserUsername}}/{{.Labels.workspace}}"}, {"label": "View workspace", "url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"}]'::jsonb,
    'Task Events',
    NULL,
    'system'::notification_template_kind,
    false
), (
    '2a74f3d3-ab09-4123-a4a5-ca238f4f65a1',
    'Task Paused',
    E'Task ''{{.Labels.task}}'' is paused',
    E'The task ''{{.Labels.task}}'' was paused ({{.Labels.pause_reason}}).',
    '[{"label": "View task", "url": "{{base_url}}/tasks/{{.UserUsername}}/{{.Labels.task_id}}"}, {"label": "View workspace", "url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"}]'::jsonb,
    'Task Events',
    NULL,
    'system'::notification_template_kind,
    true
), (
    '843ee9c3-a8fb-4846-afa9-977bec578649',
    'Task Resumed',
    E'Task ''{{.Labels.task}}'' has resumed',
    E'The task ''{{.Labels.task}}'' has resumed.',
    '[{"label": "View task", "url": "{{base_url}}/tasks/{{.UserUsername}}/{{.Labels.task_id}}"}, {"label": "View workspace", "url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"}]'::jsonb,
    'Task Events',
    NULL,
    'system'::notification_template_kind,
    true
);

-- Restore task scopes in the api_key_scope enum. Scopes stripped from API
-- keys and OAuth2 grants are not recoverable.
ALTER TYPE api_key_scope RENAME TO api_key_scope_old;

CREATE TYPE api_key_scope AS ENUM (
    'coder:all',
    'coder:application_connect',
    'aibridge_interception:create',
    'aibridge_interception:read',
    'aibridge_interception:update',
    'api_key:create',
    'api_key:delete',
    'api_key:read',
    'api_key:update',
    'assign_org_role:assign',
    'assign_org_role:create',
    'assign_org_role:delete',
    'assign_org_role:read',
    'assign_org_role:unassign',
    'assign_org_role:update',
    'assign_role:assign',
    'assign_role:read',
    'assign_role:unassign',
    'audit_log:create',
    'audit_log:read',
    'connection_log:read',
    'connection_log:update',
    'crypto_key:create',
    'crypto_key:delete',
    'crypto_key:read',
    'crypto_key:update',
    'debug_info:read',
    'deployment_config:read',
    'deployment_config:update',
    'deployment_stats:read',
    'file:create',
    'file:read',
    'group:create',
    'group:delete',
    'group:read',
    'group:update',
    'group_member:read',
    'idpsync_settings:read',
    'idpsync_settings:update',
    'inbox_notification:create',
    'inbox_notification:read',
    'inbox_notification:update',
    'license:create',
    'license:delete',
    'license:read',
    'notification_message:create',
    'notification_message:delete',
    'notification_message:read',
    'notification_message:update',
    'notification_preference:read',
    'notification_preference:update',
    'notification_template:read',
    'notification_template:update',
    'oauth2_app:create',
    'oauth2_app:delete',
    'oauth2_app:read',
    'oauth2_app:update',
    'oauth2_app_code_token:create',
    'oauth2_app_code_token:delete',
    'oauth2_app_code_token:read',
    'oauth2_app_secret:create',
    'oauth2_app_secret:delete',
    'oauth2_app_secret:read',
    'oauth2_app_secret:update',
    'organization:create',
    'organization:delete',
    'organization:read',
    'organization:update',
    'organization_member:create',
    'organization_member:delete',
    'organization_member:read',
    'organization_member:update',
    'prebuilt_workspace:delete',
    'prebuilt_workspace:update',
    'provisioner_daemon:create',
    'provisioner_daemon:delete',
    'provisioner_daemon:read',
    'provisioner_daemon:update',
    'provisioner_jobs:create',
    'provisioner_jobs:read',
    'provisioner_jobs:update',
    'replicas:read',
    'system:create',
    'system:delete',
    'system:read',
    'system:update',
    'tailnet_coordinator:create',
    'tailnet_coordinator:delete',
    'tailnet_coordinator:read',
    'tailnet_coordinator:update',
    'template:create',
    'template:delete',
    'template:read',
    'template:update',
    'template:use',
    'template:view_insights',
    'usage_event:create',
    'usage_event:read',
    'usage_event:update',
    'user:create',
    'user:delete',
    'user:read',
    'user:read_personal',
    'user:update',
    'user:update_personal',
    'user_secret:create',
    'user_secret:delete',
    'user_secret:read',
    'user_secret:update',
    'webpush_subscription:create',
    'webpush_subscription:delete',
    'webpush_subscription:read',
    'workspace:application_connect',
    'workspace:create',
    'workspace:create_agent',
    'workspace:delete',
    'workspace:delete_agent',
    'workspace:read',
    'workspace:ssh',
    'workspace:start',
    'workspace:stop',
    'workspace:update',
    'workspace_agent_devcontainers:create',
    'workspace_agent_resource_monitor:create',
    'workspace_agent_resource_monitor:read',
    'workspace_agent_resource_monitor:update',
    'workspace_dormant:application_connect',
    'workspace_dormant:create',
    'workspace_dormant:create_agent',
    'workspace_dormant:delete',
    'workspace_dormant:delete_agent',
    'workspace_dormant:read',
    'workspace_dormant:ssh',
    'workspace_dormant:start',
    'workspace_dormant:stop',
    'workspace_dormant:update',
    'workspace_proxy:create',
    'workspace_proxy:delete',
    'workspace_proxy:read',
    'workspace_proxy:update',
    'coder:workspaces.create',
    'coder:workspaces.operate',
    'coder:workspaces.delete',
    'coder:workspaces.access',
    'coder:templates.build',
    'coder:templates.author',
    'coder:apikeys.manage_self',
    'aibridge_interception:*',
    'api_key:*',
    'assign_org_role:*',
    'assign_role:*',
    'audit_log:*',
    'connection_log:*',
    'crypto_key:*',
    'debug_info:*',
    'deployment_config:*',
    'deployment_stats:*',
    'file:*',
    'group:*',
    'group_member:*',
    'idpsync_settings:*',
    'inbox_notification:*',
    'license:*',
    'notification_message:*',
    'notification_preference:*',
    'notification_template:*',
    'oauth2_app:*',
    'oauth2_app_code_token:*',
    'oauth2_app_secret:*',
    'organization:*',
    'organization_member:*',
    'prebuilt_workspace:*',
    'provisioner_daemon:*',
    'provisioner_jobs:*',
    'replicas:*',
    'system:*',
    'tailnet_coordinator:*',
    'template:*',
    'usage_event:*',
    'user:*',
    'user_secret:*',
    'webpush_subscription:*',
    'workspace:*',
    'workspace_agent_devcontainers:*',
    'workspace_agent_resource_monitor:*',
    'workspace_dormant:*',
    'workspace_proxy:*',
    'task:create',
    'task:read',
    'task:update',
    'task:delete',
    'task:*',
    'workspace:share',
    'workspace_dormant:share',
    'boundary_usage:*',
    'boundary_usage:delete',
    'boundary_usage:read',
    'boundary_usage:update',
    'workspace:update_agent',
    'workspace_dormant:update_agent',
    'chat:create',
    'chat:read',
    'chat:update',
    'chat:delete',
    'chat:*',
    'ai_seat:*',
    'ai_seat:create',
    'ai_seat:read',
    'ai_model_price:*',
    'ai_model_price:read',
    'ai_model_price:update',
    'ai_provider:*',
    'ai_provider:create',
    'ai_provider:delete',
    'ai_provider:read',
    'ai_provider:update',
    'chat:share',
    'user_skill:create',
    'user_skill:read',
    'user_skill:update',
    'user_skill:delete',
    'user_skill:*',
    'boundary_log:*',
    'boundary_log:create',
    'boundary_log:delete',
    'boundary_log:read',
    'ai_gateway_key:*',
    'ai_gateway_key:create',
    'ai_gateway_key:delete',
    'ai_gateway_key:read',
    'ai_gateway_key:update',
    'workspace_build_orchestration:*',
    'workspace_build_orchestration:create',
    'workspace_build_orchestration:delete',
    'workspace_build_orchestration:read',
    'workspace_build_orchestration:update',
    'mcp_server_config:*',
    'mcp_server_config:create',
    'mcp_server_config:read',
    'mcp_server_config:update',
    'mcp_server_config:delete',
    'mcp_server_config:share',
    'chat_model_config:*',
    'chat_model_config:create',
    'chat_model_config:read',
    'chat_model_config:update',
    'chat_model_config:delete',
    'chat_model_config:share'
);

ALTER TABLE api_keys ALTER COLUMN scopes TYPE api_key_scope[] USING (scopes::text[]::api_key_scope[]);

DROP TYPE api_key_scope_old;

-- The task audit resource type was never removed; audit history is retained
-- across the up migration.

-- Restore the task AI seat usage reason. Remapped seat events are not
-- recoverable.
ALTER TYPE ai_seat_usage_reason RENAME TO ai_seat_usage_reason_old;

CREATE TYPE ai_seat_usage_reason AS ENUM (
    'aibridge',
    'task'
);

ALTER TABLE ai_seat_state ALTER COLUMN last_event_type TYPE ai_seat_usage_reason USING (last_event_type::text::ai_seat_usage_reason);

DROP TYPE ai_seat_usage_reason_old;

