-- Remove legacy chat statuses that the chatd state machine treats as
-- invalid. 'pending', 'paused', and 'completed' are never written by the
-- backend anymore; the valid set is exactly what the state machine
-- recognizes: waiting, running, error, requires_action, interrupting.

-- Remap any historical rows to 'waiting', the idle resting state and the
-- column default. The column type is still the original chat_status here.
UPDATE chats SET status = 'waiting'
WHERE status IN ('pending', 'paused', 'completed');

-- The partial index's WHERE clause references 'pending', which is being
-- removed. The index is obsolete now that the legacy AcquireChats query
-- is gone.
DROP INDEX idx_chats_pending;

-- The view selects c.status, so it must be dropped before the column's
-- type can be altered. It is recreated verbatim below.
DROP VIEW chats_expanded;

-- Recreate the enum without the removed values using the
-- rename-create-cast-drop pattern.
ALTER TYPE chat_status RENAME TO chat_status_old;
CREATE TYPE chat_status AS ENUM (
    'waiting',
    'running',
    'error',
    'requires_action',
    'interrupting'
);
ALTER TABLE chats ALTER COLUMN status DROP DEFAULT;
ALTER TABLE chats ALTER COLUMN status TYPE chat_status USING status::text::chat_status;
ALTER TABLE chats ALTER COLUMN status SET DEFAULT 'waiting';
DROP TYPE chat_status_old;

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
    c.context_error
   FROM ((chats c
     LEFT JOIN chats root ON ((root.id = COALESCE(c.root_chat_id, c.parent_chat_id))))
     JOIN visible_users owner ON ((owner.id = c.owner_id)));
