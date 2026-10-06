-- Removes the chat automations experiment (migrations 000609 to 000611).
-- Those migrations stay unchanged because deployments already ran them.
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
				RAISE EXCEPTION 'migration 000614 could not lock the chat tables within 2 minutes';
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
