-- Adds the schema for chat automations: saved prompts that a webhook or a
-- schedule sends to a chat as their owner.
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
			LOCK TABLE chats, chat_messages, chat_queued_messages IN ACCESS EXCLUSIVE MODE;
			-- The chat_automations foreign keys need these.
			LOCK TABLE organizations, users, chat_projects, chat_model_configs, dbcrypt_keys IN SHARE ROW EXCLUSIVE MODE;
			EXIT;
		EXCEPTION WHEN lock_not_available OR deadlock_detected THEN
			IF clock_timestamp() > deadline THEN
				RAISE EXCEPTION 'migration 000642 could not lock the chat tables within 2 minutes';
			END IF;
		END;
		PERFORM pg_sleep(0.1);
	END LOOP;
	PERFORM set_config('lock_timeout', previous_lock_timeout, true);
END;
$$;

CREATE TYPE chat_automation_kind AS ENUM ('webhook', 'schedule');
CREATE TYPE chat_automation_target_mode AS ENUM ('existing_chat', 'new_chat');
CREATE TYPE chat_automation_when_busy AS ENUM ('queue', 'skip');
CREATE TYPE chat_automation_run_outcome AS ENUM ('accepted', 'skipped', 'refused');
CREATE TYPE chat_automation_webhook_auth AS ENUM ('bearer', 'github', 'standard_webhooks');

CREATE TABLE chat_automations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    owner_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 128),
    created_by_chat_id uuid REFERENCES chats (id) ON DELETE SET NULL,
    kind chat_automation_kind NOT NULL,
    enabled boolean NOT NULL DEFAULT false,
    prompt text NOT NULL,

    target_mode chat_automation_target_mode NOT NULL,
    target_chat_id uuid REFERENCES chats (id) ON DELETE SET NULL,
    when_busy chat_automation_when_busy,
    new_chat_model_config_id uuid,
    reasoning_effort chat_reasoning_effort,
    project_id uuid REFERENCES chat_projects (id) ON DELETE SET NULL,

    webhook_auth chat_automation_webhook_auth,
    webhook_secret text,
    webhook_secret_key_id text REFERENCES dbcrypt_keys (active_key_digest),
    webhook_secret_version bigint NOT NULL DEFAULT 0,

    schedule_cron text,
    schedule_time_zone text,
    schedule_next_run_at timestamptz,
    schedule_revision bigint NOT NULL DEFAULT 1,

    last_run_at timestamptz,
    last_run_outcome chat_automation_run_outcome,
    last_run_reason text,
    last_run_chat_id uuid,
    last_run_unaccepted_count integer NOT NULL DEFAULT 0 CHECK (last_run_unaccepted_count >= 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    -- Keeps the new-chat model in the automation's organization, matching
    -- the composite foreign keys on chat_*_model_overrides.
    CONSTRAINT chat_automations_new_chat_model_config_fkey
        FOREIGN KEY (organization_id, new_chat_model_config_id)
        REFERENCES chat_model_configs (organization_id, id),
    -- Each kind sets only its own columns. A webhook without a secret
    -- could not verify any request.
    CONSTRAINT chat_automations_kind_shape CHECK (
        (kind = 'webhook'
            AND webhook_auth IS NOT NULL AND webhook_secret IS NOT NULL
            AND schedule_cron IS NULL AND schedule_time_zone IS NULL AND schedule_next_run_at IS NULL)
        OR (kind = 'schedule'
            AND webhook_auth IS NULL AND webhook_secret IS NULL AND webhook_secret_key_id IS NULL
            AND schedule_cron IS NOT NULL AND schedule_time_zone IS NOT NULL)
    ),
    -- Each target mode sets only its own columns. target_chat_id is not
    -- required for existing_chat: deleting the target chat sets it to NULL.
    CONSTRAINT chat_automations_target_shape CHECK (
        (target_mode = 'existing_chat'
            AND when_busy IS NOT NULL AND new_chat_model_config_id IS NULL)
        OR (target_mode = 'new_chat'
            AND target_chat_id IS NULL AND when_busy IS NULL AND new_chat_model_config_id IS NOT NULL)
    ),
    CONSTRAINT chat_automations_project_target_mode
        CHECK (project_id IS NULL OR target_mode = 'new_chat'),
    CONSTRAINT chat_automations_last_run_shape
        CHECK ((last_run_at IS NULL) = (last_run_outcome IS NULL))
);

COMMENT ON TABLE chat_automations IS 'Saved prompts that a webhook or a schedule sends to a chat as their owner.';
COMMENT ON COLUMN chat_automations.created_by_chat_id IS 'Chat whose agent created the automation. NULL when a person created it or after that chat is deleted.';
COMMENT ON COLUMN chat_automations.target_chat_id IS 'Target of an existing_chat automation. NULL after the target chat is deleted.';
COMMENT ON COLUMN chat_automations.project_id IS 'Project for the chats a new_chat automation creates. NULL when it has none or after the project is deleted.';
COMMENT ON COLUMN chat_automations.webhook_secret IS 'Bearer token or HMAC key that authenticates webhook requests.';
COMMENT ON COLUMN chat_automations.webhook_secret_key_id IS 'The ID of the key used to encrypt the webhook secret. If this is NULL, the webhook secret is not encrypted.';
COMMENT ON COLUMN chat_automations.webhook_secret_version IS 'Incremented on every rotation, so a run can refuse a request that an older secret verified.';
COMMENT ON COLUMN chat_automations.schedule_next_run_at IS 'The next occurrence to publish. NULL when none is pending.';
COMMENT ON COLUMN chat_automations.schedule_revision IS 'Incremented on every schedule change, so a run can detect a schedule that changed after it was read.';
COMMENT ON COLUMN chat_automations.last_run_chat_id IS 'Chat of the latest run. No foreign key: the chat can be deleted.';
COMMENT ON COLUMN chat_automations.last_run_unaccepted_count IS 'Skipped and refused runs since the last accepted run. An accepted run resets it to zero.';

-- A composite foreign key cannot keep these references in the automation's
-- organization: ON DELETE SET NULL would also clear organization_id, and
-- column-list SET NULL needs PostgreSQL 15.
CREATE FUNCTION enforce_chat_automation_organization() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	-- Validate a reference only when it changes. Chat deletion runs one ON
	-- DELETE SET NULL update per reference, and the other, unchanged
	-- reference may point at a chat the same statement is deleting.
	IF NEW.target_chat_id IS NOT NULL AND (
		TG_OP = 'INSERT'
		OR NEW.target_chat_id IS DISTINCT FROM OLD.target_chat_id
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
	) AND NOT EXISTS (
		SELECT 1 FROM chats
		WHERE id = NEW.created_by_chat_id AND organization_id = NEW.organization_id
	) THEN
		RAISE EXCEPTION 'creating chat % is not in organization %', NEW.created_by_chat_id, NEW.organization_id
			USING ERRCODE = 'check_violation',
			      CONSTRAINT = 'chat_automations_chat_organization';
	END IF;
	IF NEW.project_id IS NOT NULL AND (
		TG_OP = 'INSERT'
		OR NEW.project_id IS DISTINCT FROM OLD.project_id
	) AND NOT EXISTS (
		SELECT 1 FROM chat_projects
		WHERE id = NEW.project_id AND organization_id = NEW.organization_id
	) THEN
		RAISE EXCEPTION 'project % is not in organization %', NEW.project_id, NEW.organization_id
			USING ERRCODE = 'check_violation',
			      CONSTRAINT = 'chat_automations_project_organization';
	END IF;
	RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_enforce_chat_automation_organization
    BEFORE INSERT OR UPDATE OF target_chat_id, created_by_chat_id, project_id ON chat_automations
    FOR EACH ROW EXECUTE FUNCTION enforce_chat_automation_organization();

CREATE INDEX chat_automations_org_owner_idx ON chat_automations (organization_id, owner_id);
-- Serves the per-user count across organizations and the cleanup when a
-- user is deleted.
CREATE INDEX chat_automations_owner_id_idx ON chat_automations (owner_id);
CREATE INDEX chat_automations_due_idx ON chat_automations (schedule_next_run_at) WHERE kind = 'schedule' AND enabled;
-- Chat and project deletion enforce ON DELETE SET NULL through these columns.
CREATE INDEX chat_automations_target_chat_id_idx ON chat_automations (target_chat_id) WHERE target_chat_id IS NOT NULL;
CREATE INDEX chat_automations_created_by_chat_id_idx ON chat_automations (created_by_chat_id) WHERE created_by_chat_id IS NOT NULL;
CREATE INDEX chat_automations_project_id_idx ON chat_automations (project_id) WHERE project_id IS NOT NULL;

-- Coder soft-deletes users, so the cascade on owner_id never fires. Delete a
-- deleted user's automations here, like their other personal resources.
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

        -- Remove their chat automations and the webhook secrets in them.
        -- chat_automations.owner_id has ON DELETE CASCADE, but soft-delete
        -- does not remove the users row so the FK cascade never fires.
        DELETE FROM chat_automations
        WHERE owner_id = OLD.id;
    END IF;
    RETURN NEW;
END;
$$;

ALTER TABLE chats
    ADD COLUMN automation_id uuid,
    ADD COLUMN manage_automations_enabled boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN chats.automation_id IS 'Automation that created this chat. No foreign key: provenance stays after the automation is deleted.';
COMMENT ON COLUMN chats.manage_automations_enabled IS 'Whether the agent of this chat can manage automations.';

ALTER TABLE chat_messages
    ADD COLUMN automation_id uuid,
    ADD COLUMN input_id uuid,
    ADD COLUMN prompt_hooks_pending boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN chat_messages.automation_id IS 'Automation that sent this message. No foreign key: provenance stays after the automation is deleted.';
COMMENT ON COLUMN chat_messages.input_id IS 'Webhook delivery or schedule occurrence that sent this message.';
COMMENT ON COLUMN chat_messages.prompt_hooks_pending IS 'Set when the message was saved without running the user_prompt_submit hooks. chatd runs them when the message starts a turn, then clears the flag.';

ALTER TABLE chat_queued_messages
    ADD COLUMN automation_id uuid,
    ADD COLUMN input_id uuid,
    ADD COLUMN prompt_hooks_pending boolean NOT NULL DEFAULT false,
    -- The queue is small, so validating this check under the lock is cheap.
    ADD CONSTRAINT chat_queued_messages_automation_input_check CHECK (
        (automation_id IS NULL AND input_id IS NULL)
        OR (automation_id IS NOT NULL AND input_id IS NOT NULL)
    );

COMMENT ON COLUMN chat_queued_messages.automation_id IS 'Automation that queued this message. No foreign key: provenance stays after the automation is deleted.';
COMMENT ON COLUMN chat_queued_messages.input_id IS 'Webhook delivery or schedule occurrence that queued this message. Set together with automation_id.';
COMMENT ON COLUMN chat_queued_messages.prompt_hooks_pending IS 'Set when the message was queued without running the user_prompt_submit hooks. chatd runs them when the message starts a turn, then clears the flag.';

CREATE INDEX chats_automation_idx ON chats (automation_id) WHERE automation_id IS NOT NULL;
CREATE INDEX chat_messages_automation_idx ON chat_messages (automation_id) WHERE automation_id IS NOT NULL;
CREATE INDEX chat_queued_messages_automation_idx ON chat_queued_messages (automation_id) WHERE automation_id IS NOT NULL;

ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:delete';

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
