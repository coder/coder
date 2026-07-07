-- Restore the removed legacy statuses in their original enum order. Data
-- is not restored: the removed values were unused, so rows remapped to
-- 'waiting' by the up migration stay 'waiting'.
DROP VIEW chats_expanded;

ALTER TYPE chat_status RENAME TO chat_status_old;
CREATE TYPE chat_status AS ENUM (
    'waiting',
    'pending',
    'running',
    'paused',
    'completed',
    'error',
    'requires_action',
    'interrupting'
);
ALTER TABLE chats ALTER COLUMN status DROP DEFAULT;
ALTER TABLE chats ALTER COLUMN status TYPE chat_status USING status::text::chat_status;
ALTER TABLE chats ALTER COLUMN status SET DEFAULT 'waiting';
DROP TYPE chat_status_old;

CREATE INDEX idx_chats_pending ON chats USING btree (status) WHERE (status = 'pending'::chat_status);

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
