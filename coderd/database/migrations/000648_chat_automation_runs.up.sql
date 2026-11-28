-- A query on chats_expanded locks the view before chats, so dropping the
-- view first takes the locks in that order too, and a query that already
-- holds the view cannot wait for chats while this migration waits for the
-- view. The view is recreated at the end.
DROP VIEW chats_expanded;

-- Provenance of the chats and messages that automation runs create. No
-- foreign key: the provenance outlives the automation.
ALTER TABLE chats ADD COLUMN automation_id uuid;
ALTER TABLE chat_messages ADD COLUMN automation_id uuid;

COMMENT ON COLUMN chats.automation_id IS 'Chat automation whose run created this chat. Not a foreign key: it stays after the automation is deleted.';
COMMENT ON COLUMN chat_messages.automation_id IS 'Chat automation whose run sent this message. Not a foreign key: it stays after the automation is deleted.';

-- Serves the overlap check of new_chat schedules.
CREATE INDEX chats_automation_id_idx ON chats (automation_id) WHERE automation_id IS NOT NULL;

ALTER TABLE chat_automations ADD COLUMN last_run_unaccepted_count integer NOT NULL DEFAULT 0;

COMMENT ON COLUMN chat_automations.last_run_unaccepted_count IS 'Skipped and refused runs since the last accepted run.';

ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:run';

-- Recreate chats_expanded, dropped at the top, with automation_id.
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
    c.title_updated_at,
    c.automation_id
   FROM ((chats c
     LEFT JOIN chats root ON ((root.id = COALESCE(c.root_chat_id, c.parent_chat_id))))
     JOIN visible_users owner ON ((owner.id = c.owner_id)));
