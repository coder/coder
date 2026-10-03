-- Rollback for the chatd core state machine foundation migration.

-- 1. Recreate chats_expanded without the new chat fields. We must drop
-- the view first because the subsequent column drops would fail with
-- "view depends on column".
DROP VIEW IF EXISTS chats_expanded;

-- 2. Drop the worker acquisition candidates index.
DROP INDEX IF EXISTS idx_chats_worker_acquisition_candidates;

-- 3. Drop the retry state trigger and function.
DROP TRIGGER IF EXISTS trigger_sync_chat_retry_state ON chats;
DROP FUNCTION IF EXISTS sync_chat_retry_state();

-- 4. Drop the queue version triggers and function.
DROP TRIGGER IF EXISTS trigger_bump_chat_queue_version_on_queued_message_delete ON chat_queued_messages;
DROP TRIGGER IF EXISTS trigger_bump_chat_queue_version_on_queued_message_update ON chat_queued_messages;
DROP TRIGGER IF EXISTS trigger_bump_chat_queue_version_on_queued_message_insert ON chat_queued_messages;
DROP FUNCTION IF EXISTS bump_chat_queue_version_on_queued_message_change();

-- 5. Drop the message revision triggers and functions.
DROP TRIGGER IF EXISTS trigger_update_chat_history_after_message_update ON chat_messages;
DROP TRIGGER IF EXISTS trigger_update_chat_history_after_message_insert ON chat_messages;
DROP TRIGGER IF EXISTS trigger_set_chat_message_revision_on_update ON chat_messages;
DROP TRIGGER IF EXISTS trigger_set_chat_message_revision_on_insert ON chat_messages;
DROP FUNCTION IF EXISTS update_chat_history_after_message_update();
DROP FUNCTION IF EXISTS update_chat_history_after_message_insert();
-- The pre-split function name is kept here for backward compatibility
-- with environments that may have applied an earlier draft of the up
-- migration. DROP FUNCTION IF EXISTS is a no-op if the function is
-- absent.
DROP FUNCTION IF EXISTS update_chat_history_after_message_changes();
DROP FUNCTION IF EXISTS set_chat_message_revision_before();
DROP FUNCTION IF EXISTS set_chat_message_revision();

-- 6. Drop chat_heartbeats (and its index by association).
DROP TABLE IF EXISTS chat_heartbeats;

-- 7. Drop chat_queued_messages.position and its default sequence, plus
-- created_by.
ALTER TABLE chat_queued_messages
    ALTER COLUMN position DROP DEFAULT;
ALTER TABLE chat_queued_messages
    DROP COLUMN IF EXISTS position,
    DROP COLUMN IF EXISTS created_by;
DROP SEQUENCE IF EXISTS chat_queued_messages_position_seq;

-- 8. Drop chat_messages.revision.
ALTER TABLE chat_messages
    DROP COLUMN IF EXISTS revision;

-- 9. Drop the new chats columns.
ALTER TABLE chats
    DROP COLUMN IF EXISTS snapshot_version,
    DROP COLUMN IF EXISTS history_version,
    DROP COLUMN IF EXISTS queue_version,
    DROP COLUMN IF EXISTS generation_attempt,
    DROP COLUMN IF EXISTS retry_state,
    DROP COLUMN IF EXISTS retry_state_version,
    DROP COLUMN IF EXISTS runner_id,
    DROP COLUMN IF EXISTS requires_action_deadline_at;

-- 10. Recreate chats_expanded with the pre-migration field list.
CREATE VIEW chats_expanded AS
SELECT
    c.id,
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
    c.archived,
    c.last_error,
    c.mode,
    c.mcp_server_ids,
    c.labels,
    c.build_id,
    c.agent_id,
    c.pin_order,
    c.last_read_message_id,
    c.last_injected_context,
    c.dynamic_tools,
    c.organization_id,
    c.plan_mode,
    c.client_type,
    c.last_turn_summary,
    COALESCE(root.user_acl, c.user_acl) AS user_acl,
    COALESCE(root.group_acl, c.group_acl) AS group_acl,
    owner.username AS owner_username,
    owner.name AS owner_name
FROM
    chats c
    LEFT JOIN chats root ON root.id = COALESCE(c.root_chat_id, c.parent_chat_id)
    JOIN visible_users owner ON owner.id = c.owner_id;

-- 11. The `interrupting` chat_status enum value is intentionally left
-- in place. Postgres does not support dropping a single enum value
-- without recreating the entire type, which would require rewriting
-- every chat row and is unsafe inside a transactional rollback.
