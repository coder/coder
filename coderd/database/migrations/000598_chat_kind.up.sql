-- chats.kind classifies each chat row explicitly instead of inferring
-- "subagent" from parent_chat_id being set. Existing rows with a parent are
-- subagents; every other row is a user chat. 'root' rows are per
-- (owner_id, organization_id) tree roots and are created lazily later.
CREATE TYPE chat_kind AS ENUM ('root', 'chat', 'subagent');

ALTER TABLE chats
    ADD COLUMN kind chat_kind NOT NULL DEFAULT 'chat';

UPDATE chats SET kind = 'subagent' WHERE parent_chat_id IS NOT NULL;

-- Subagents always carry root_chat_id; rows without a parent never do.
UPDATE chats SET root_chat_id = parent_chat_id
    WHERE parent_chat_id IS NOT NULL AND root_chat_id IS NULL;
UPDATE chats SET root_chat_id = NULL
    WHERE parent_chat_id IS NULL AND root_chat_id IS NOT NULL;

ALTER TABLE chats
    DROP CONSTRAINT chat_acl_only_on_root_chats,
    DROP CONSTRAINT chats_pin_order_parent_check;

ALTER TABLE chats
    -- Only non-subagent chats hold their own ACL; subagents inherit it
    -- through chats_expanded.
    ADD CONSTRAINT chat_acl_only_on_root_chats
        CHECK (
            kind <> 'subagent'
            OR (user_acl = '{}'::jsonb AND group_acl = '{}'::jsonb)
        ),
    ADD CONSTRAINT chats_pin_order_parent_check
        CHECK (pin_order = 0 OR kind = 'chat'),
    ADD CONSTRAINT chats_kind_subagent_root_check
        CHECK ((kind = 'subagent') = (root_chat_id IS NOT NULL)),
    ADD CONSTRAINT chats_kind_subagent_parent_check
        CHECK (kind <> 'subagent' OR parent_chat_id IS NOT NULL),
    ADD CONSTRAINT chats_kind_root_parentless_check
        CHECK (kind <> 'root' OR (parent_chat_id IS NULL AND root_chat_id IS NULL));

-- At most one tree root per owner per organization.
CREATE UNIQUE INDEX chats_one_tree_root_per_owner_org
    ON chats (owner_id, organization_id)
    WHERE kind = 'root';

DROP INDEX idx_chats_auto_archive_candidates;
CREATE INDEX idx_chats_auto_archive_candidates
    ON chats (created_at)
    WHERE archived = false
      AND pin_order = 0
      AND kind = 'chat';

DROP INDEX idx_chats_worker_acquisition_candidates;
CREATE INDEX idx_chats_worker_acquisition_candidates ON chats
    ((kind = 'subagent'), status, updated_at, id)
    WHERE archived = false;

-- Recreate chats_expanded: its explicit column list hides new columns
-- otherwise. The ACL join keys on root_chat_id alone; it is set exactly on
-- subagent rows.
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
    c.kind,
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
     LEFT JOIN chats root ON ((root.id = c.root_chat_id)))
     JOIN visible_users owner ON ((owner.id = c.owner_id)));
