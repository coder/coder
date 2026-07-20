-- Recreate chats_expanded because adding a chats column changes its row type.
DROP VIEW IF EXISTS chats_expanded;

ALTER TABLE chats ADD COLUMN hook_allowed_tools jsonb;
ALTER TABLE chat_messages ADD COLUMN turn_id uuid;
ALTER TABLE chat_queued_messages
    ADD COLUMN turn_id uuid,
    ADD COLUMN hook_prefix jsonb,
    ADD COLUMN hook_allowed_tools jsonb;

COMMENT ON COLUMN chats.hook_allowed_tools IS 'Hook-enforced tool names; NULL means unrestricted. Later policies only narrow.';
COMMENT ON COLUMN chat_queued_messages.hook_allowed_tools IS 'Queued prompt hook policy; NULL means no policy.';

-- Create-time prompt dispatches precede the chat row, so chat_id has no
-- foreign key.
CREATE TABLE chat_hook_dispatches (
    id uuid PRIMARY KEY,
    chat_id uuid NOT NULL,
    event text NOT NULL,
    turn_id uuid,
    tool_use_id text,
    owner_id uuid NOT NULL,
    workspace_id uuid,
    started_at timestamptz NOT NULL,
    finished_at timestamptz,
    result text NOT NULL DEFAULT 'pending',
    http_status integer,
    decision text,
    input_override jsonb,
    original_input jsonb,
    model_context text,
    user_message text,
    allowed_tools jsonb,
    end_chat boolean,
    error text,
    decision_reason text,
    effects_applied_at timestamptz,
    tool_name text
);

COMMENT ON TABLE chat_hook_dispatches IS 'Lifecycle hook attempts keyed by dispatch_id (JWT jti).';

CREATE INDEX idx_chat_hook_dispatches_chat_id ON chat_hook_dispatches (chat_id);
CREATE INDEX idx_chat_hook_dispatches_started_at ON chat_hook_dispatches (started_at);

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
    c.hook_allowed_tools
   FROM ((chats c
     LEFT JOIN chats root ON ((root.id = COALESCE(c.root_chat_id, c.parent_chat_id))))
     JOIN visible_users owner ON ((owner.id = c.owner_id)));
