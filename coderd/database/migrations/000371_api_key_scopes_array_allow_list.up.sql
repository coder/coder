-- Extend api_key_scope enum with low-level <resource>:<action> values derived from RBACPermissions
-- Generated via: go run ./scripts/generate_api_key_scope_enum
-- Begin enum extensions
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'aibridge_interception:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'aibridge_interception:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'aibridge_interception:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'api_key:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'api_key:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'api_key:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'api_key:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'assign_org_role:assign';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'assign_org_role:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'assign_org_role:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'assign_org_role:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'assign_org_role:unassign';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'assign_org_role:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'assign_role:assign';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'assign_role:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'assign_role:unassign';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'audit_log:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'audit_log:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'connection_log:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'connection_log:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'crypto_key:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'crypto_key:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'crypto_key:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'crypto_key:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'debug_info:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'deployment_config:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'deployment_config:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'deployment_stats:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'file:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'file:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'group:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'group:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'group:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'group:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'group_member:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'idpsync_settings:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'idpsync_settings:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'inbox_notification:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'inbox_notification:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'inbox_notification:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'license:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'license:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'license:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'notification_message:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'notification_message:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'notification_message:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'notification_message:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'notification_preference:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'notification_preference:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'notification_template:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'notification_template:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'oauth2_app:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'oauth2_app:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'oauth2_app:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'oauth2_app:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'oauth2_app_code_token:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'oauth2_app_code_token:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'oauth2_app_code_token:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'oauth2_app_secret:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'oauth2_app_secret:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'oauth2_app_secret:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'oauth2_app_secret:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'organization:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'organization:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'organization:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'organization:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'organization_member:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'organization_member:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'organization_member:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'organization_member:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'prebuilt_workspace:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'prebuilt_workspace:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'provisioner_daemon:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'provisioner_daemon:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'provisioner_daemon:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'provisioner_daemon:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'provisioner_jobs:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'provisioner_jobs:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'provisioner_jobs:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'replicas:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'system:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'system:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'system:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'system:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'tailnet_coordinator:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'tailnet_coordinator:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'tailnet_coordinator:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'tailnet_coordinator:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'template:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'template:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'template:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'template:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'template:use';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'template:view_insights';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'usage_event:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'usage_event:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'usage_event:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user:read_personal';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user:update_personal';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_secret:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_secret:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_secret:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_secret:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'webpush_subscription:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'webpush_subscription:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'webpush_subscription:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace:application_connect';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace:create_agent';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace:delete_agent';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace:ssh';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace:start';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace:stop';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_agent_devcontainers:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_agent_resource_monitor:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_agent_resource_monitor:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_agent_resource_monitor:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_dormant:application_connect';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_dormant:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_dormant:create_agent';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_dormant:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_dormant:delete_agent';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_dormant:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_dormant:ssh';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_dormant:start';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_dormant:stop';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_dormant:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_proxy:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_proxy:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_proxy:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_proxy:update';
-- End enum extensions

-- Purge old API keys to speed up the migration for large deployments.
-- Note: that problem should be solved in coderd once PR 20863 is released:
-- https://github.com/coder/coder/blob/main/coderd/database/dbpurge/dbpurge.go#L85

DELETE FROM api_keys WHERE expires_at < NOW() - INTERVAL '7 days';

-- Add new columns without defaults; backfill; then enforce NOT NULL
ALTER TABLE api_keys
    ADD COLUMN scopes api_key_scope[],
    ADD COLUMN allow_list text[];

-- Backfill existing rows for compatibility
UPDATE api_keys SET
    scopes = ARRAY[scope::api_key_scope],
    allow_list = ARRAY['*:*'];

-- Enforce NOT NULL and drop legacy single-scope column
ALTER TABLE api_keys
    ALTER COLUMN scopes SET NOT NULL,
    ALTER COLUMN allow_list SET NOT NULL,
    DROP COLUMN scope;
