-- Drop task views first to remove dependencies on tables and columns.
DROP VIEW IF EXISTS tasks_with_status;
DROP VIEW IF EXISTS workspaces_expanded;
DROP VIEW IF EXISTS template_version_with_user;
DROP VIEW IF EXISTS workspace_build_with_user;

-- Drop task tables.
DROP TABLE IF EXISTS task_snapshots;
DROP TABLE IF EXISTS task_workspace_apps;
DROP TABLE IF EXISTS tasks;

-- Drop task status enum type.
DROP TYPE IF EXISTS task_status;

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
    COALESCE(( SELECT jsonb_object_agg(acl.key, jsonb_build_object('name', COALESCE(g.name, ''::text), 'avatar_url', COALESCE(g.avatar_url, ''::text))) AS jsonb_object_agg
           FROM (jsonb_each(workspaces.group_acl) acl(key, value)
             LEFT JOIN groups g ON ((g.id = (acl.key)::uuid)))), '{}'::jsonb) AS group_acl_display_info,
    COALESCE(( SELECT jsonb_object_agg(acl.key, jsonb_build_object('name', COALESCE(vu.name, ''::text), 'avatar_url', COALESCE(vu.avatar_url, ''::text))) AS jsonb_object_agg
           FROM (jsonb_each(workspaces.user_acl) acl(key, value)
             LEFT JOIN visible_users vu ON ((vu.id = (acl.key)::uuid)))), '{}'::jsonb) AS user_acl_display_info
   FROM (((workspaces
     JOIN visible_users ON ((workspaces.owner_id = visible_users.id)))
     JOIN organizations ON ((workspaces.organization_id = organizations.id)))
     JOIN templates ON ((workspaces.template_id = templates.id)));

COMMENT ON VIEW workspaces_expanded IS 'Joins in the display name information such as username, avatar, and organization name.';

-- Delete task notification templates and preferences. The delete cascades
-- to inbox_notifications and notification_messages: historical task
-- notifications are intentionally removed with the feature.
DELETE FROM notification_templates
WHERE id IN (
    'bd4b7168-d05e-4e19-ad0f-3593b77aa90f', -- Task Working
    'd4a6271c-cced-4ed0-84ad-afd02a9c7799', -- Task Idle
    '8c5a4d12-9f7e-4b3a-a1c8-6e4f2d9b5a7c', -- Task Completed
    '3b7e8f1a-4c2d-49a6-b5e9-7f3a1c8d6b4e', -- Task Failed
    '2a74f3d3-ab09-4123-a4a5-ca238f4f65a1', -- Task Paused
    '843ee9c3-a8fb-4846-afa9-977bec578649'  -- Task Resumed
);

DELETE FROM user_configs
WHERE key = 'preference_task_notification_alert_dismissed';

-- Remove task permissions from custom roles.
UPDATE custom_roles
SET site_permissions = COALESCE((
    SELECT jsonb_agg(elem)
    FROM jsonb_array_elements(site_permissions) elem
    WHERE elem->>'resource_type' != 'task'
), '[]'::jsonb)
WHERE site_permissions @> '[{"resource_type": "task"}]';

UPDATE custom_roles
SET org_permissions = COALESCE((
    SELECT jsonb_agg(elem)
    FROM jsonb_array_elements(org_permissions) elem
    WHERE elem->>'resource_type' != 'task'
), '[]'::jsonb)
WHERE org_permissions @> '[{"resource_type": "task"}]';

UPDATE custom_roles
SET user_permissions = COALESCE((
    SELECT jsonb_agg(elem)
    FROM jsonb_array_elements(user_permissions) elem
    WHERE elem->>'resource_type' != 'task'
), '[]'::jsonb)
WHERE user_permissions @> '[{"resource_type": "task"}]';

UPDATE custom_roles
SET member_permissions = COALESCE((
    SELECT jsonb_agg(elem)
    FROM jsonb_array_elements(member_permissions) elem
    WHERE elem->>'resource_type' != 'task'
), '[]'::jsonb)
WHERE member_permissions @> '[{"resource_type": "task"}]';

-- Remove task scopes from persisted API keys and OAuth2 grants so scope
-- expansion cannot hit the deleted task scope definitions. Keys and grants
-- left with no other scope are revoked outright.
DELETE FROM api_keys
WHERE EXISTS (SELECT 1 FROM unnest(scopes) s WHERE s::text LIKE 'task:%')
  AND NOT EXISTS (SELECT 1 FROM unnest(scopes) s WHERE s::text NOT LIKE 'task:%');

UPDATE api_keys
SET scopes = ARRAY(SELECT s FROM unnest(scopes) s WHERE s::text NOT LIKE 'task:%')
WHERE EXISTS (SELECT 1 FROM unnest(scopes) s WHERE s::text LIKE 'task:%');

-- Remove task entries from API key allow lists; loading a key reparses every
-- entry against the RBAC resource catalog, which no longer knows task. Keys
-- whose allow list only referenced tasks are revoked outright because an
-- empty allow list would mean unrestricted access.
DELETE FROM api_keys
WHERE NOT EXISTS (SELECT 1 FROM unnest(allow_list) e WHERE e NOT LIKE 'task:%');

UPDATE api_keys
SET allow_list = ARRAY(SELECT e FROM unnest(allow_list) e WHERE e NOT LIKE 'task:%')
WHERE EXISTS (SELECT 1 FROM unnest(allow_list) e WHERE e LIKE 'task:%');

DELETE FROM oauth2_provider_app_codes
WHERE EXISTS (SELECT 1 FROM unnest(string_to_array(scope, ' ')) w WHERE w LIKE 'task:%')
  AND NOT EXISTS (SELECT 1 FROM unnest(string_to_array(scope, ' ')) w WHERE w NOT LIKE 'task:%' AND w <> '');

UPDATE oauth2_provider_app_codes
SET scope = array_to_string(ARRAY(SELECT w FROM unnest(string_to_array(scope, ' ')) w WHERE w NOT LIKE 'task:%' AND w <> ''), ' ')
WHERE EXISTS (SELECT 1 FROM unnest(string_to_array(scope, ' ')) w WHERE w LIKE 'task:%');

DELETE FROM oauth2_provider_app_tokens
WHERE EXISTS (SELECT 1 FROM unnest(string_to_array(scope, ' ')) w WHERE w LIKE 'task:%')
  AND NOT EXISTS (SELECT 1 FROM unnest(string_to_array(scope, ' ')) w WHERE w NOT LIKE 'task:%' AND w <> '');

UPDATE oauth2_provider_app_tokens
SET scope = array_to_string(ARRAY(SELECT w FROM unnest(string_to_array(scope, ' ')) w WHERE w NOT LIKE 'task:%' AND w <> ''), ' ')
WHERE EXISTS (SELECT 1 FROM unnest(string_to_array(scope, ' ')) w WHERE w LIKE 'task:%');

-- Neutralize apps whose allow list named only task scopes. Stripping to an
-- empty string would read as "no allowlist configured" (unrestricted), so
-- store a whitespace-only allowlist, a configured value that grants nothing.
UPDATE oauth2_provider_apps
SET scope = ' '
WHERE EXISTS (SELECT 1 FROM unnest(string_to_array(scope, ' ')) w WHERE w LIKE 'task:%')
  AND NOT EXISTS (SELECT 1 FROM unnest(string_to_array(scope, ' ')) w WHERE w NOT LIKE 'task:%' AND w <> '');

UPDATE oauth2_provider_apps
SET scope = array_to_string(ARRAY(SELECT w FROM unnest(string_to_array(scope, ' ')) w WHERE w NOT LIKE 'task:%' AND w <> ''), ' ')
WHERE EXISTS (SELECT 1 FROM unnest(string_to_array(scope, ' ')) w WHERE w LIKE 'task:%');

-- Remove task scopes from the api_key_scope enum.
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

-- Task audit history is retained: audit logs are subject to retention
-- policies, so the task value stays in the resource_type enum as a
-- historical audit value.

-- Remove the task AI seat usage reason. Seats consumed by tasks stay
-- consumed; their last event type is remapped to aibridge.
UPDATE ai_seat_state SET last_event_type = 'aibridge' WHERE last_event_type::text = 'task';

ALTER TYPE ai_seat_usage_reason RENAME TO ai_seat_usage_reason_old;

CREATE TYPE ai_seat_usage_reason AS ENUM (
    'aibridge'
);

ALTER TABLE ai_seat_state ALTER COLUMN last_event_type TYPE ai_seat_usage_reason USING (last_event_type::text::ai_seat_usage_reason);

DROP TYPE ai_seat_usage_reason_old;

