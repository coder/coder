-- Title provenance. Automatic title generation may only replace a
-- 'fallback' title; a 'user' title is never replaced automatically.
-- Persisting intent (rather than comparing title text) makes the rule
-- hold even when a chosen title happens to equal the derived fallback,
-- or when a user renames back to an earlier value.
CREATE TYPE chat_title_source AS ENUM (
    'fallback',
    'generated',
    'user'
);

COMMENT ON TYPE chat_title_source IS 'Provenance of chats.title. fallback: derived from the first prompt at creation. generated: written by automatic title generation. user: supplied by the user at creation or by rename.';

ALTER TABLE chats
    ADD COLUMN title_source chat_title_source NOT NULL DEFAULT 'fallback';

COMMENT ON COLUMN chats.title_source IS 'Provenance of title. Automatic title generation only replaces a fallback title.';

-- Refresh chats_expanded to include the new chat column. The gentest
-- TestViewSubsetChat requires every chats column to appear in the view.
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
    c.title_source
   FROM ((chats c
     LEFT JOIN chats root ON ((root.id = COALESCE(c.root_chat_id, c.parent_chat_id))))
     JOIN visible_users owner ON ((owner.id = c.owner_id)));
