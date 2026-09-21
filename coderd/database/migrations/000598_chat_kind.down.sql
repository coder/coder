DROP VIEW IF EXISTS chats_expanded;

-- The kind CHECK constraints are dropped before any rows change so the SET NULL
-- referential actions below cannot violate them.
ALTER TABLE chats
    DROP CONSTRAINT chat_acl_only_on_root_chats,
    DROP CONSTRAINT chats_pin_order_parent_check,
    DROP CONSTRAINT chats_kind_subagent_root_check,
    DROP CONSTRAINT chats_kind_subagent_parent_check,
    DROP CONSTRAINT chats_kind_root_parentless_check;

-- Named children and tree roots do not exist without the kind column:
-- remove subagents spawned by roots, detach named children, then remove
-- the roots.
DELETE FROM chats
WHERE root_chat_id IN (SELECT id FROM chats WHERE kind = 'root');
UPDATE chats SET parent_chat_id = NULL WHERE kind = 'chat';
DELETE FROM chats WHERE kind = 'root';

DROP INDEX IF EXISTS chats_one_tree_root_per_owner_org;

DROP INDEX idx_chats_auto_archive_candidates;
DROP INDEX idx_chats_worker_acquisition_candidates;

ALTER TABLE chats DROP COLUMN kind;

DROP TYPE chat_kind;

ALTER TABLE chats
    ADD CONSTRAINT chat_acl_only_on_root_chats
        CHECK (
            (parent_chat_id IS NULL AND root_chat_id IS NULL)
            OR (
                user_acl = '{}'::jsonb
                AND group_acl = '{}'::jsonb
            )
        ),
    ADD CONSTRAINT chats_pin_order_parent_check
        CHECK (pin_order = 0 OR parent_chat_id IS NULL);

CREATE INDEX idx_chats_auto_archive_candidates
    ON chats (created_at)
    WHERE archived = false
      AND pin_order = 0
      AND parent_chat_id IS NULL;

CREATE INDEX idx_chats_worker_acquisition_candidates ON chats
    ((parent_chat_id IS NULL), status, updated_at, id)
    WHERE archived = false;

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
    c.compaction_requested_at
   FROM ((chats c
     LEFT JOIN chats root ON ((root.id = COALESCE(c.root_chat_id, c.parent_chat_id))))
     JOIN visible_users owner ON ((owner.id = c.owner_id)));
