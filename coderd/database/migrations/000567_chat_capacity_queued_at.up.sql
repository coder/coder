-- Set while a chat waits for a concurrent-agent capacity slot; the
-- worker refuses acquisition when the chat's pool is full. NULL means
-- the chat is not waiting for capacity.
ALTER TABLE chats
    ADD COLUMN capacity_queued_at TIMESTAMPTZ;

CREATE INDEX idx_chats_capacity_queued_at ON chats (capacity_queued_at)
    WHERE capacity_queued_at IS NOT NULL;

-- Keeps the capacity slot count an index-only scan over the handful of
-- generating chats instead of a walk over all historical chats.
CREATE INDEX idx_chats_capacity_active ON chats (parent_chat_id)
    WHERE archived = false
      AND status IN ('running', 'interrupting')
      AND worker_id IS NOT NULL
      AND runner_id IS NOT NULL;

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
    c.capacity_queued_at
   FROM ((chats c
     LEFT JOIN chats root ON ((root.id = COALESCE(c.root_chat_id, c.parent_chat_id))))
     JOIN visible_users owner ON ((owner.id = c.owner_id)));
