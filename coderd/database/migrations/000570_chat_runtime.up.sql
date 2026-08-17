CREATE TYPE chat_runtime AS ENUM ('coder', 'claude_code');

COMMENT ON TYPE chat_runtime IS 'Generation runtime backing a chat: coder chats use the built-in LLM pipeline, claude_code chats delegate turns to a Claude Code agent running inside the bound workspace.';

ALTER TABLE chats ADD COLUMN runtime chat_runtime NOT NULL DEFAULT 'coder';
ALTER TABLE chats ADD COLUMN runtime_state jsonb;

COMMENT ON COLUMN chats.runtime IS 'Generation runtime for this chat. Immutable after creation.';
COMMENT ON COLUMN chats.runtime_state IS 'Runtime-specific persistent state, e.g. the ACP session ID and adapter capabilities for claude_code chats.';

ALTER TABLE chats ALTER COLUMN last_model_config_id DROP NOT NULL;

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
    c.runtime,
    c.runtime_state
   FROM ((chats c
     LEFT JOIN chats root ON ((root.id = COALESCE(c.root_chat_id, c.parent_chat_id))))
     JOIN visible_users owner ON ((owner.id = c.owner_id)));

CREATE TABLE chat_runtime_configs (
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    runtime chat_runtime NOT NULL,
    template_id uuid NOT NULL REFERENCES templates (id) ON DELETE CASCADE,
    enabled boolean NOT NULL DEFAULT true,
    model text NOT NULL DEFAULT '',
    permission_mode text NOT NULL DEFAULT '',
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, runtime)
);

COMMENT ON TABLE chat_runtime_configs IS 'Per-organization admin configuration for external chat runtimes, e.g. which template backs Claude Code chats.';
COMMENT ON COLUMN chat_runtime_configs.model IS 'Optional model identifier pinned for the runtime. Empty means the runtime default.';
COMMENT ON COLUMN chat_runtime_configs.permission_mode IS 'Optional permission mode the runtime agent runs with. Empty means the runtime default.';
