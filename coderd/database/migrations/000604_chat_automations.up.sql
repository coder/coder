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
	IF NEW.target_chat_id IS NOT NULL AND NOT EXISTS (
		SELECT 1 FROM chats
		WHERE id = NEW.target_chat_id AND organization_id = NEW.organization_id
	) THEN
		RAISE EXCEPTION 'target chat % is not in organization %', NEW.target_chat_id, NEW.organization_id
			USING ERRCODE = 'check_violation',
			      CONSTRAINT = 'chat_automations_chat_organization';
	END IF;
	IF NEW.created_by_chat_id IS NOT NULL AND NOT EXISTS (
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

-- Recreate chats_expanded: its explicit column list hides new columns otherwise.
DROP VIEW IF EXISTS chats_expanded;

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
    c.automation_id
   FROM ((chats c
     LEFT JOIN chats root ON ((root.id = COALESCE(c.root_chat_id, c.parent_chat_id))))
     JOIN visible_users owner ON ((owner.id = c.owner_id)));
