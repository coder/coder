-- Snapshot of the owner's custom prompt as it was shown to the lifecycle
-- hook policy with the chat's most recent user_prompt_submit admission.
-- When hooks are enabled, generation injects this snapshot instead of
-- re-reading the live per-user config, so the model can never receive a
-- custom prompt a policy was not shown. NULL means no admission has
-- recorded a snapshot; hook-enabled generation then injects nothing.
ALTER TABLE chats
    ADD COLUMN admitted_custom_prompt text;

COMMENT ON COLUMN chats.admitted_custom_prompt IS 'Owner custom prompt admitted with the most recent user_prompt_submit lifecycle event. Injected verbatim by generation while hooks are enabled; NULL when no admission recorded one.';

-- Queued sends are admitted at queue time but generate later, possibly
-- after other admissions changed the chat-level snapshot. Each queued row
-- carries its own admission snapshot; promotion copies it to the chat row
-- in the same transaction that moves the message into history, so every
-- turn generates with the value its own admission event was shown.
ALTER TABLE chat_queued_messages
    ADD COLUMN admitted_custom_prompt text;

COMMENT ON COLUMN chat_queued_messages.admitted_custom_prompt IS 'Owner custom prompt admitted with this queued message''s user_prompt_submit lifecycle event. Copied to chats.admitted_custom_prompt when the row is promoted into history; NULL when no admission recorded one.';

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
    c.admitted_custom_prompt
   FROM ((chats c
     LEFT JOIN chats root ON ((root.id = COALESCE(c.root_chat_id, c.parent_chat_id))))
     JOIN visible_users owner ON ((owner.id = c.owner_id)));
