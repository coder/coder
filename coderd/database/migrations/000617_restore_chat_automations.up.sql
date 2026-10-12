-- Restores the chat automations experiment after 000615 and 000616 removed
-- it. This is the 000615 down migration followed by the 000616 down
-- migration: it recreates the 000611 schema with empty tables and adds the
-- chat_automation:* values back to api_key_scope. Data deleted by 000615
-- and 000616 does not come back.
--
-- Take every table lock this migration needs before changing anything. See
-- 000609_chat_automations.up.sql for the reasoning. chat_automations does
-- not exist yet, so it is not locked here.
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
			LOCK TABLE chats, chat_messages, chat_queued_messages IN ACCESS EXCLUSIVE MODE;
			-- The chat_automations foreign keys need these.
			LOCK TABLE organizations, users, chat_model_configs IN SHARE ROW EXCLUSIVE MODE;
			EXIT;
		EXCEPTION WHEN lock_not_available OR deadlock_detected THEN
			IF clock_timestamp() > deadline THEN
				RAISE EXCEPTION 'migration 000617 could not lock the chat tables within 2 minutes';
			END IF;
		END;
		PERFORM pg_sleep(0.1);
	END LOOP;
	PERFORM set_config('lock_timeout', previous_lock_timeout, true);
END;
$$;

CREATE TYPE chat_automation_kind AS ENUM ('webhook', 'schedule');
CREATE TYPE chat_automation_target_mode AS ENUM ('existing_chat', 'new_chat');
CREATE TYPE chat_automation_webhook_use AS ENUM ('single', 'multi');
CREATE TYPE chat_automation_when_busy AS ENUM ('queue', 'skip');

CREATE TABLE chat_automations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    owner_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name text NOT NULL,
    created_by_chat_id uuid REFERENCES chats (id) ON DELETE SET NULL,
    kind chat_automation_kind NOT NULL,
    enabled boolean NOT NULL DEFAULT false,
    target_mode chat_automation_target_mode NOT NULL,
    target_chat_id uuid REFERENCES chats (id) ON DELETE SET NULL,
    new_chat_model_config_id uuid,
    reasoning_effort chat_reasoning_effort,
    when_busy chat_automation_when_busy,
    webhook_use chat_automation_webhook_use,
    webhook_secret_hash bytea,
    webhook_secret_version bigint NOT NULL DEFAULT 0,
    webhook_consumed_at timestamptz,
    prompt text NOT NULL,
    schedule_cron text,
    schedule_time_zone text,
    schedule_revision bigint NOT NULL DEFAULT 1,
    schedule_next_run_at timestamptz,
    queue_generation bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chat_automations_name_length CHECK (char_length(name) BETWEEN 1 AND 128),
    -- Keeps the new-chat model in the automation's organization, matching
    -- the composite foreign keys on chat_*_model_overrides.
    CONSTRAINT chat_automations_new_chat_model_config_fkey
        FOREIGN KEY (organization_id, new_chat_model_config_id)
        REFERENCES chat_model_configs (organization_id, id),
    CONSTRAINT chat_automations_target_shape CHECK (
        (target_mode = 'existing_chat' AND new_chat_model_config_id IS NULL AND when_busy IS NOT NULL)
        OR (target_mode = 'new_chat' AND target_chat_id IS NULL AND new_chat_model_config_id IS NOT NULL AND when_busy IS NULL)
    ),
    CONSTRAINT chat_automations_kind_shape CHECK (
        (kind = 'webhook' AND webhook_use IS NOT NULL AND schedule_cron IS NULL)
        OR (kind = 'schedule' AND webhook_use IS NULL AND webhook_secret_hash IS NULL AND schedule_cron IS NOT NULL AND schedule_time_zone IS NOT NULL)
    )
);

COMMENT ON TABLE chat_automations IS 'Owner-authored webhook or schedule triggers that deliver a prompt to an existing chat or a new chat.';
COMMENT ON COLUMN chat_automations.target_chat_id IS 'Target chat for existing_chat automations. Set to NULL when the target chat is deleted.';
COMMENT ON COLUMN chat_automations.webhook_secret_hash IS 'Hash of the webhook bearer secret. The plaintext secret is never stored.';
COMMENT ON COLUMN chat_automations.webhook_secret_version IS 'Incremented each time the webhook secret is rotated.';
COMMENT ON COLUMN chat_automations.webhook_consumed_at IS 'Single-use webhook marker. Set once when the webhook is consumed and never reset.';
COMMENT ON COLUMN chat_automations.schedule_revision IS 'Incremented whenever the schedule changes, so work computed from an older schedule can be detected as stale.';
COMMENT ON COLUMN chat_automations.schedule_next_run_at IS 'Schedule cursor: the next occurrence to fire. NULL when no occurrence is pending.';
COMMENT ON COLUMN chat_automations.queue_generation IS 'Incremented to invalidate queued messages this automation delivered earlier; queued rows carry the generation they were created with.';

-- A composite foreign key cannot keep these chats in the automation's
-- organization: ON DELETE SET NULL would also clear organization_id, and
-- column-list SET NULL needs PostgreSQL 15.
CREATE FUNCTION enforce_chat_automation_chat_organization() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	-- Validate a reference only when it or organization_id changes. Chat
	-- deletion runs one ON DELETE SET NULL update per reference, and the
	-- other, unchanged reference may point at a chat the same statement is
	-- deleting.
	IF NEW.target_chat_id IS NOT NULL AND (
		TG_OP = 'INSERT'
		OR NEW.target_chat_id IS DISTINCT FROM OLD.target_chat_id
		OR NEW.organization_id IS DISTINCT FROM OLD.organization_id
	) AND NOT EXISTS (
		SELECT 1 FROM chats
		WHERE id = NEW.target_chat_id AND organization_id = NEW.organization_id
	) THEN
		RAISE EXCEPTION 'target chat % is not in organization %', NEW.target_chat_id, NEW.organization_id
			USING ERRCODE = 'check_violation',
			      CONSTRAINT = 'chat_automations_chat_organization';
	END IF;
	IF NEW.created_by_chat_id IS NOT NULL AND (
		TG_OP = 'INSERT'
		OR NEW.created_by_chat_id IS DISTINCT FROM OLD.created_by_chat_id
		OR NEW.organization_id IS DISTINCT FROM OLD.organization_id
	) AND NOT EXISTS (
		SELECT 1 FROM chats
		WHERE id = NEW.created_by_chat_id AND organization_id = NEW.organization_id
	) THEN
		RAISE EXCEPTION 'creating chat % is not in organization %', NEW.created_by_chat_id, NEW.organization_id
			USING ERRCODE = 'check_violation',
			      CONSTRAINT = 'chat_automations_chat_organization';
	END IF;
	RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_enforce_chat_automation_chat_organization
    BEFORE INSERT OR UPDATE OF organization_id, target_chat_id, created_by_chat_id ON chat_automations
    FOR EACH ROW EXECUTE FUNCTION enforce_chat_automation_chat_organization();

CREATE INDEX chat_automations_org_owner_idx ON chat_automations (organization_id, owner_id);
-- Serves owner lookups without an organization, including the users cascade.
CREATE INDEX chat_automations_owner_id_idx ON chat_automations (owner_id);
-- Chat deletion enforces ON DELETE SET NULL through these columns.
CREATE INDEX chat_automations_target_chat_id_idx ON chat_automations (target_chat_id) WHERE target_chat_id IS NOT NULL;
CREATE INDEX chat_automations_created_by_chat_id_idx ON chat_automations (created_by_chat_id) WHERE created_by_chat_id IS NOT NULL;
CREATE INDEX chat_automations_due_idx ON chat_automations (schedule_next_run_at) WHERE kind = 'schedule' AND enabled;

-- Provenance columns record which automation input produced a row. They
-- have no foreign key to chat_automations by design: provenance must
-- survive the automation's deletion.
ALTER TABLE chat_queued_messages
    ADD COLUMN automation_id uuid,
    ADD COLUMN input_id uuid,
    ADD COLUMN queue_generation bigint,
    ADD CONSTRAINT chat_queued_messages_automation_shape CHECK (
        (automation_id IS NULL AND input_id IS NULL AND queue_generation IS NULL)
        OR (automation_id IS NOT NULL AND input_id IS NOT NULL AND queue_generation IS NOT NULL)
    );

COMMENT ON COLUMN chat_queued_messages.automation_id IS 'Automation that queued this message. No foreign key by design.';
COMMENT ON COLUMN chat_queued_messages.input_id IS 'Automation input (webhook delivery or schedule occurrence) that queued this message.';
COMMENT ON COLUMN chat_queued_messages.queue_generation IS 'chat_automations.queue_generation at queue time. A lower value than the automation''s current generation marks the message stale.';

CREATE INDEX chat_queued_messages_automation_idx ON chat_queued_messages (automation_id) WHERE automation_id IS NOT NULL;

ALTER TABLE chat_messages
    ADD COLUMN automation_id uuid,
    ADD COLUMN input_id uuid;

COMMENT ON COLUMN chat_messages.automation_id IS 'Automation that delivered this message. No foreign key by design.';
COMMENT ON COLUMN chat_messages.input_id IS 'Automation input (webhook delivery or schedule occurrence) that delivered this message.';

CREATE INDEX chat_messages_automation_idx ON chat_messages (automation_id) WHERE automation_id IS NOT NULL;

ALTER TABLE chats
    ADD COLUMN automation_id uuid;

COMMENT ON COLUMN chats.automation_id IS 'Automation that created this chat. No foreign key by design.';

CREATE INDEX chats_automation_idx ON chats (automation_id) WHERE automation_id IS NOT NULL;

ALTER TABLE chats
    ADD COLUMN manage_automations_enabled boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN chats.manage_automations_enabled IS 'Interim per-chat switch that offers the manage_automations tool. Only the chat owner may change it after creation.';

-- Recreate chats_expanded, dropped at the top of this migration, with the
-- restored columns.
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
    c.title_updated_at,
    c.automation_id,
    c.manage_automations_enabled
   FROM ((chats c
     LEFT JOIN chats root ON ((root.id = COALESCE(c.root_chat_id, c.parent_chat_id))))
     JOIN visible_users owner ON ((owner.id = c.owner_id)));


-- Restore chat_automation scopes in the api_key_scope enum, in their
-- original position. Revoked keys and grants, stripped scopes, and removed
-- custom role permissions are not recoverable, and neither is a deleted
-- chat-automations experiment rule.
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
    'chat_automation:*',
    'chat_automation:create',
    'chat_automation:read',
    'chat_automation:update',
    'chat_automation:delete',
    'chat_project_memory:*',
    'chat_project_memory:create',
    'chat_project_memory:read',
    'chat_project_memory:delete'
);

ALTER TABLE api_keys ALTER COLUMN scopes TYPE api_key_scope[] USING (scopes::text[]::api_key_scope[]);

DROP TYPE api_key_scope_old;
