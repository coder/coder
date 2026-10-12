-- Removes the chat automations experiment again: the 000615 up migration
-- followed by the 000616 up migration.
--
-- Take every table lock this migration needs before changing anything,
-- retrying short waits so that concurrent chatd traffic cannot deadlock
-- with it. See 000609_chat_automations.up.sql for the reasoning.
DO $$
DECLARE
	previous_lock_timeout text := current_setting('lock_timeout');
	deadline timestamptz := clock_timestamp() + interval '2 minutes';
BEGIN
	LOOP
		BEGIN
			-- Well below the default deadlock_timeout of 1s, so this
			-- transaction gives up before PostgreSQL can choose an
			-- application transaction as the deadlock victim.
			PERFORM set_config('lock_timeout', '100ms', true);
			-- chats_expanded is recreated below. DROP VIEW locks only the
			-- view, while LOCK TABLE on the view would also lock users (via
			-- visible_users) in ACCESS EXCLUSIVE mode.
			DROP VIEW IF EXISTS chats_expanded;
			LOCK TABLE chats, chat_messages, chat_queued_messages, chat_automations IN ACCESS EXCLUSIVE MODE;
			-- Dropping chat_automations drops its foreign keys, and PostgreSQL
			-- takes ACCESS EXCLUSIVE on every referenced table to do that.
			LOCK TABLE organizations, users, chat_model_configs IN ACCESS EXCLUSIVE MODE;
			EXIT;
		EXCEPTION WHEN lock_not_available OR deadlock_detected THEN
			IF clock_timestamp() > deadline THEN
				RAISE EXCEPTION 'migration 000617 down could not lock the chat tables within 2 minutes';
			END IF;
		END;
		PERFORM pg_sleep(0.1);
	END LOOP;
	PERFORM set_config('lock_timeout', previous_lock_timeout, true);
END;
$$;

-- Messages that automations queued cannot be delivered without the
-- automation that owns them. Ordinary queued messages stay. The queue
-- version trigger sets chats.queue_version to chats.snapshot_version for
-- each affected chat. snapshot_version is not advanced: coderd restarts to
-- run this migration, every stream loop then bootstraps from an empty local
-- state and refetches the queue, and the chat worker reads the queue from
-- the database whenever it promotes a message.
DELETE FROM chat_queued_messages WHERE automation_id IS NOT NULL;

DROP INDEX chats_automation_idx;
ALTER TABLE chats
    DROP COLUMN manage_automations_enabled,
    DROP COLUMN automation_id;

DROP INDEX chat_messages_automation_idx;
ALTER TABLE chat_messages
    DROP COLUMN input_id,
    DROP COLUMN automation_id;

DROP INDEX chat_queued_messages_automation_idx;
ALTER TABLE chat_queued_messages
    DROP CONSTRAINT chat_queued_messages_automation_shape,
    DROP COLUMN queue_generation,
    DROP COLUMN input_id,
    DROP COLUMN automation_id;

DROP TABLE chat_automations;
DROP FUNCTION enforce_chat_automation_chat_organization();

DROP TYPE chat_automation_when_busy;
DROP TYPE chat_automation_webhook_use;
DROP TYPE chat_automation_target_mode;
DROP TYPE chat_automation_kind;

-- Recreate chats_expanded, dropped at the top of this migration, without
-- the removed columns.
CREATE VIEW chats_expanded AS
 SELECT c.id,
    c.owner_id,
    c.workspace_id,
    c.title,
    c.status,
    c.worker_id,
    c.started_at,
    c.heartbeat_at,
    c.created_at,
    c.updated_at,
    c.parent_chat_id,
    c.root_chat_id,
    c.last_model_config_id,
    c.last_reasoning_effort,
    c.archived,
    c.last_error,
    c.mode,
    c.mcp_server_ids,
    c.labels,
    c.build_id,
    c.agent_id,
    c.pin_order,
    c.last_read_message_id,
    c.dynamic_tools,
    c.organization_id,
    c.project_id,
    c.plan_mode,
    c.client_type,
    c.last_turn_summary,
    c.summary,
    c.summary_generated_at,
    c.snapshot_version,
    c.history_version,
    c.queue_version,
    c.generation_attempt,
    c.retry_state,
    c.retry_state_version,
    c.runner_id,
    c.requires_action_deadline_at,
    COALESCE(root.user_acl, c.user_acl) AS user_acl,
    COALESCE(root.group_acl, c.group_acl) AS group_acl,
    owner.username AS owner_username,
    owner.name AS owner_name,
    c.context_aggregate_hash,
    c.context_dirty_since,
    c.context_dirty_resources,
    c.context_error,
    c.compaction_requested_at,
    c.title_source,
    c.title_updated_at
   FROM ((chats c
     LEFT JOIN chats root ON ((root.id = COALESCE(c.root_chat_id, c.parent_chat_id))))
     JOIN visible_users owner ON ((owner.id = c.owner_id)));


-- Removes chat_automation credentials and permissions left by the chat
-- automations experiment, following 000595_remove_task_permissions. This
-- migration touches api_keys, custom_roles, site_configs and the
-- oauth2_provider_* tables only. It does not touch chat tables, so it does
-- not need the lock retry block from 000615.

-- LIKE treats an unescaped underscore as a wildcard, so the patterns below
-- escape it to match the chat_automation prefix exactly.

-- Remove chat_automation permissions from custom roles.
UPDATE custom_roles
SET site_permissions = COALESCE((
    SELECT jsonb_agg(elem)
    FROM jsonb_array_elements(site_permissions) elem
    WHERE elem->>'resource_type' != 'chat_automation'
), '[]'::jsonb)
WHERE site_permissions @> '[{"resource_type": "chat_automation"}]';

UPDATE custom_roles
SET org_permissions = COALESCE((
    SELECT jsonb_agg(elem)
    FROM jsonb_array_elements(org_permissions) elem
    WHERE elem->>'resource_type' != 'chat_automation'
), '[]'::jsonb)
WHERE org_permissions @> '[{"resource_type": "chat_automation"}]';

UPDATE custom_roles
SET user_permissions = COALESCE((
    SELECT jsonb_agg(elem)
    FROM jsonb_array_elements(user_permissions) elem
    WHERE elem->>'resource_type' != 'chat_automation'
), '[]'::jsonb)
WHERE user_permissions @> '[{"resource_type": "chat_automation"}]';

UPDATE custom_roles
SET member_permissions = COALESCE((
    SELECT jsonb_agg(elem)
    FROM jsonb_array_elements(member_permissions) elem
    WHERE elem->>'resource_type' != 'chat_automation'
), '[]'::jsonb)
WHERE member_permissions @> '[{"resource_type": "chat_automation"}]';

-- Remove chat_automation scopes from persisted API keys and OAuth2 grants so
-- scope expansion cannot hit the deleted scope definitions. Keys and grants
-- left with no other scope are revoked outright.
DELETE FROM api_keys
WHERE EXISTS (SELECT 1 FROM unnest(scopes) s WHERE s::text LIKE 'chat\_automation:%')
  AND NOT EXISTS (SELECT 1 FROM unnest(scopes) s WHERE s::text NOT LIKE 'chat\_automation:%');

UPDATE api_keys
SET scopes = ARRAY(SELECT s FROM unnest(scopes) s WHERE s::text NOT LIKE 'chat\_automation:%')
WHERE EXISTS (SELECT 1 FROM unnest(scopes) s WHERE s::text LIKE 'chat\_automation:%');

-- Remove chat_automation entries from API key allow lists; loading a key
-- reparses every entry against the RBAC resource catalog, which no longer
-- knows chat_automation. The allow list cannot be empty, so keys that only
-- referenced chat automations are revoked.
DELETE FROM api_keys
WHERE EXISTS (SELECT 1 FROM unnest(allow_list) e WHERE e LIKE 'chat\_automation:%')
  AND NOT EXISTS (SELECT 1 FROM unnest(allow_list) e WHERE e NOT LIKE 'chat\_automation:%');

UPDATE api_keys
SET allow_list = ARRAY(SELECT e FROM unnest(allow_list) e WHERE e NOT LIKE 'chat\_automation:%')
WHERE EXISTS (SELECT 1 FROM unnest(allow_list) e WHERE e LIKE 'chat\_automation:%');

DELETE FROM oauth2_provider_app_codes
WHERE EXISTS (SELECT 1 FROM unnest(string_to_array(scope, ' ')) w WHERE w LIKE 'chat\_automation:%')
  AND NOT EXISTS (SELECT 1 FROM unnest(string_to_array(scope, ' ')) w WHERE w NOT LIKE 'chat\_automation:%' AND w <> '');

UPDATE oauth2_provider_app_codes
SET scope = array_to_string(ARRAY(SELECT w FROM unnest(string_to_array(scope, ' ')) w WHERE w NOT LIKE 'chat\_automation:%' AND w <> ''), ' ')
WHERE EXISTS (SELECT 1 FROM unnest(string_to_array(scope, ' ')) w WHERE w LIKE 'chat\_automation:%');

DELETE FROM oauth2_provider_app_tokens
WHERE EXISTS (SELECT 1 FROM unnest(string_to_array(scope, ' ')) w WHERE w LIKE 'chat\_automation:%')
  AND NOT EXISTS (SELECT 1 FROM unnest(string_to_array(scope, ' ')) w WHERE w NOT LIKE 'chat\_automation:%' AND w <> '');

UPDATE oauth2_provider_app_tokens
SET scope = array_to_string(ARRAY(SELECT w FROM unnest(string_to_array(scope, ' ')) w WHERE w NOT LIKE 'chat\_automation:%' AND w <> ''), ' ')
WHERE EXISTS (SELECT 1 FROM unnest(string_to_array(scope, ' ')) w WHERE w LIKE 'chat\_automation:%');

-- Neutralize apps whose allow list named only chat_automation scopes.
-- Stripping to an empty string would read as "no allowlist configured"
-- (unrestricted), so store a whitespace-only allowlist, a configured value
-- that grants nothing.
UPDATE oauth2_provider_apps
SET scope = ' '
WHERE EXISTS (SELECT 1 FROM unnest(string_to_array(scope, ' ')) w WHERE w LIKE 'chat\_automation:%')
  AND NOT EXISTS (SELECT 1 FROM unnest(string_to_array(scope, ' ')) w WHERE w NOT LIKE 'chat\_automation:%' AND w <> '');

UPDATE oauth2_provider_apps
SET scope = array_to_string(ARRAY(SELECT w FROM unnest(string_to_array(scope, ' ')) w WHERE w NOT LIKE 'chat\_automation:%' AND w <> ''), ' ')
WHERE EXISTS (SELECT 1 FROM unnest(string_to_array(scope, ' ')) w WHERE w LIKE 'chat\_automation:%');

-- Delete the stored runtime rule for the removed chat-automations
-- experiment. The server would otherwise keep listing it as ignored.
DELETE FROM site_configs WHERE key = 'experiment_rule:chat-automations';

-- Remove chat_automation scopes from the api_key_scope enum. Recreating the
-- type, instead of altering it, keeps every remaining value usable in the
-- same migration transaction.
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
    'chat_model_config:share',
    'chat_project:*',
    'chat_project:create',
    'chat_project:read',
    'chat_project:update',
    'chat_project:delete',
    'chat_project_memory:*',
    'chat_project_memory:create',
    'chat_project_memory:read',
    'chat_project_memory:delete'
);

ALTER TABLE api_keys ALTER COLUMN scopes TYPE api_key_scope[] USING (scopes::text[]::api_key_scope[]);

DROP TYPE api_key_scope_old;

-- Chat automation audit history is retained: audit logs are subject to
-- retention policies, so the chat_automation value stays in the
-- resource_type enum as a historical audit value.
