-- Move last_turn_summary off the hot chats row into a side table so the
-- asynchronous display-summary write stops contending with FOR NO KEY UPDATE
-- state transitions. Follows the chat_heartbeats decoupling pattern: child-row
-- writes take a key-share FK lock on chats instead of a conflicting row lock.
CREATE TABLE chat_last_turn_summaries (
    chat_id uuid PRIMARY KEY REFERENCES chats(id) ON DELETE CASCADE,
    last_turn_summary text,
    history_version bigint NOT NULL
);

COMMENT ON TABLE chat_last_turn_summaries IS
    'Cached display-only summary of the latest completed turn, written asynchronously after an LLM label call. Split from chats so summary writes do not lock the hot chat row. history_version is the freshness watermark the summary was generated for.';

-- Backfill existing summaries, carrying the chat's current history_version as
-- the initial watermark. A NULL summary is represented by an absent row.
INSERT INTO chat_last_turn_summaries (chat_id, last_turn_summary, history_version)
SELECT id, last_turn_summary, history_version
FROM chats
WHERE last_turn_summary IS NOT NULL;

-- Drop the view before the column it references, drop the column, then recreate
-- the view sourcing last_turn_summary from the side table.
DROP VIEW IF EXISTS chats_expanded;

ALTER TABLE chats DROP COLUMN last_turn_summary;

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
    lts.last_turn_summary,
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
    c.compaction_requested_at
   FROM (((chats c
     LEFT JOIN chats root ON ((root.id = COALESCE(c.root_chat_id, c.parent_chat_id))))
     JOIN visible_users owner ON ((owner.id = c.owner_id)))
     LEFT JOIN chat_last_turn_summaries lts ON ((lts.chat_id = c.id)));
