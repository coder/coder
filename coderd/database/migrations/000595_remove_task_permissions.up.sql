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
WHERE NOT EXISTS (SELECT 1 FROM unnest(scopes) s WHERE s::text NOT LIKE 'task:%');

UPDATE api_keys
SET scopes = ARRAY(SELECT s FROM unnest(scopes) s WHERE s::text NOT LIKE 'task:%')
WHERE EXISTS (SELECT 1 FROM unnest(scopes) s WHERE s::text LIKE 'task:%');

-- Remove task entries from API key allow lists; loading a key reparses every
-- entry against the RBAC resource catalog, which no longer knows task. The
-- allow list cannot be empty, so keys that only referenced tasks are revoked.
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
