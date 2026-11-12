-- Take every table lock this migration needs before changing anything. See
-- 000609_chat_automations.up.sql for the reasoning.
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
			-- The scope cleanup below deletes API keys, which cascades to
			-- oauth2_provider_app_tokens. The application deletes in the
			-- opposite order: a token delete fires a trigger that deletes
			-- its API key.
			LOCK TABLE chats, chat_messages, chat_queued_messages, chat_automations, api_keys, oauth2_provider_app_tokens, custom_roles IN ACCESS EXCLUSIVE MODE;
			-- Dropping chat_automations drops its foreign keys, and PostgreSQL
			-- takes ACCESS EXCLUSIVE on every referenced table to do that.
			LOCK TABLE organizations, users, chat_projects, chat_model_configs, dbcrypt_keys IN ACCESS EXCLUSIVE MODE;
			EXIT;
		EXCEPTION WHEN lock_not_available OR deadlock_detected THEN
			IF clock_timestamp() > deadline THEN
				RAISE EXCEPTION 'migration 000642 down could not lock the chat tables within 2 minutes';
			END IF;
		END;
		PERFORM pg_sleep(0.1);
	END LOOP;
	PERFORM set_config('lock_timeout', previous_lock_timeout, true);
END;
$$;

-- A message with prompt_hooks_pending has not run its user_prompt_submit
-- hooks, which can deny or rewrite it, and the flag is dropped below. The
-- previous release would deliver such a message without the hooks.
--
-- A message in chat_messages is part of a chat's history, and deleting it
-- would rewrite a conversation. Stop instead, so that nothing is erased.
DO $$
DECLARE
	pending bigint;
BEGIN
	SELECT count(*) INTO pending FROM chat_messages WHERE prompt_hooks_pending;
	IF pending > 0 THEN
		-- The guidance is in the message: the migrator prints no HINT.
		RAISE EXCEPTION 'migration 000642 down: % chat messages still wait for their prompt hooks. Keep this release running until chatd has started these turns, then run the down migration again.', pending;
	END IF;
END;
$$;

-- A queued message has not entered the history yet, so delete it.
DELETE FROM chat_queued_messages WHERE automation_id IS NOT NULL OR prompt_hooks_pending;

-- Dropping a column drops its indexes.
ALTER TABLE chats
    DROP COLUMN manage_automations_enabled,
    DROP COLUMN automation_id;

ALTER TABLE chat_messages
    DROP COLUMN prompt_hooks_pending,
    DROP COLUMN input_id,
    DROP COLUMN automation_id;

ALTER TABLE chat_queued_messages
    DROP CONSTRAINT chat_queued_messages_automation_input_check,
    DROP COLUMN prompt_hooks_pending,
    DROP COLUMN input_id,
    DROP COLUMN automation_id;

-- The function body names chat_automations, which is dropped below.
CREATE OR REPLACE FUNCTION delete_deleted_user_resources() RETURNS trigger
    LANGUAGE plpgsql
AS $$
DECLARE
BEGIN
    IF (NEW.deleted) THEN
        -- Remove their api_keys.
        DELETE FROM api_keys
        WHERE user_id = OLD.id;

        -- Remove their user_links.
        -- Their login_type is preserved in the users table.
        -- Matching this user back to the link can still be done by their
        -- email if the account is undeleted. Although that is not a guarantee.
        DELETE FROM user_links
        WHERE user_id = OLD.id;

        -- Remove their user_secrets.
        -- user_secrets.user_id has ON DELETE CASCADE, but soft-delete
        -- does not remove the users row so the FK cascade never fires.
        DELETE FROM user_secrets
        WHERE user_id = OLD.id;

        -- Remove their user AI provider keys.
        -- user_ai_provider_keys.user_id has ON DELETE CASCADE, but soft-delete
        -- does not remove the users row so the FK cascade never fires.
        DELETE FROM user_ai_provider_keys
        WHERE user_id = OLD.id;

        -- Remove their organization memberships.
        -- This also triggers group membership cleanup via
        -- trigger_delete_group_members_on_org_member_delete.
        DELETE FROM organization_members
        WHERE user_id = OLD.id;

        -- Remove their personal skills.
        -- skills.user_id has ON DELETE CASCADE, but soft-delete
        -- does not remove the users row so the FK cascade never fires.
        DELETE FROM skills
        WHERE user_id = OLD.id;
    END IF;
    RETURN NEW;
END;
$$;

DROP TABLE chat_automations;
DROP FUNCTION enforce_chat_automation_organization();

DROP TYPE chat_automation_webhook_auth;
DROP TYPE chat_automation_run_outcome;
DROP TYPE chat_automation_when_busy;
DROP TYPE chat_automation_target_mode;
DROP TYPE chat_automation_kind;

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

-- Remove the chat_automation scopes from the api_key_scope enum. PostgreSQL
-- cannot drop an enum value, so recreate the type as 000616 did. LIKE treats
-- an unescaped underscore as a wildcard, so the patterns escape it.
--
-- A binary without the chat_automation resource cannot load a custom role
-- permission or an API key allow list entry that names it, and the scopes
-- column cannot hold a removed value. Remove all three, as 000616 did. A key
-- left with no scope or with an empty allow list is revoked. OAuth2 grants
-- need no cleanup: they hold external scopes only, and these five are
-- internal.
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

DELETE FROM api_keys
WHERE EXISTS (SELECT 1 FROM unnest(allow_list) e WHERE e LIKE 'chat\_automation:%')
  AND NOT EXISTS (SELECT 1 FROM unnest(allow_list) e WHERE e NOT LIKE 'chat\_automation:%');

UPDATE api_keys
SET allow_list = ARRAY(SELECT e FROM unnest(allow_list) e WHERE e NOT LIKE 'chat\_automation:%')
WHERE EXISTS (SELECT 1 FROM unnest(allow_list) e WHERE e LIKE 'chat\_automation:%');

DELETE FROM api_keys
WHERE EXISTS (SELECT 1 FROM unnest(scopes) s WHERE s::text LIKE 'chat\_automation:%')
  AND NOT EXISTS (SELECT 1 FROM unnest(scopes) s WHERE s::text NOT LIKE 'chat\_automation:%');

UPDATE api_keys
SET scopes = ARRAY(SELECT s FROM unnest(scopes) s WHERE s::text NOT LIKE 'chat\_automation:%')
WHERE EXISTS (SELECT 1 FROM unnest(scopes) s WHERE s::text LIKE 'chat\_automation:%');

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
    'chat_project_memory:delete',
    'organization_skill:*',
    'organization_skill:create',
    'organization_skill:read',
    'organization_skill:update',
    'organization_skill:delete',
    'organization_skill:share',
    'chat_project:share',
    'workspace_secret:*',
    'workspace_secret:create',
    'workspace_secret:read',
    'workspace_secret:read_secret',
    'workspace_secret:update'
);

ALTER TABLE api_keys ALTER COLUMN scopes TYPE api_key_scope[] USING (scopes::text[]::api_key_scope[]);

DROP TYPE api_key_scope_old;
