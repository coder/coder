CREATE UNIQUE INDEX idx_ai_agents_origin ON ai_agents (origin_type, origin_id) WHERE NOT deleted;

ALTER TABLE workspaces
    DROP CONSTRAINT workspaces_ai_agent_id_fkey,
    ADD CONSTRAINT workspaces_ai_agent_id_fkey
        FOREIGN KEY (ai_agent_id) REFERENCES ai_agents (user_id);

ALTER TABLE workspace_agents
    DROP CONSTRAINT workspace_agents_ai_agent_id_fkey,
    ADD CONSTRAINT workspace_agents_ai_agent_id_fkey
        FOREIGN KEY (ai_agent_id) REFERENCES ai_agents (user_id);

ALTER TABLE ai_sandboxes
    DROP CONSTRAINT ai_sandboxes_ai_agent_id_fkey,
    ADD CONSTRAINT ai_sandboxes_ai_agent_id_fkey
        FOREIGN KEY (ai_agent_id) REFERENCES ai_agents (user_id);

COMMENT ON COLUMN ai_sandbox_sessions.ai_agent_id IS 'AI agent identity snapshot. Not a foreign key to ai_agents; retained after identity revocation and cleanup.';

ALTER TABLE ai_sandboxes DROP COLUMN occupancy_count;

DROP VIEW IF EXISTS chats_expanded;

ALTER TABLE chats
    DROP CONSTRAINT chats_occupancy_only_on_root_chats,
    DROP COLUMN occupancy_count;

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

-- The ledger rows written for agents the identity code created are left in
-- place. They are valid entries with valid folds, and deleting them would
-- discard a record to undo a schema change. Re-applying the up migration skips
-- what it already wrote.
ALTER TABLE ai_agent_ledger DROP COLUMN creation_time;

COMMENT ON TABLE ai_agent_ledger IS 'Current state of each AI agent identity. Three absences are deliberate. There is no workspace or sandbox reference, because an AI agent''s identity is independent of where it runs and may outlive any particular sandbox. There is no execution state, because an identity and a run of it are different things, and a schema merging them forecloses reconstituting an AI agent from a previous session. There is no creation time, because the journal records when this row came to exist and a second copy could disagree with the first.';

ALTER TABLE ai_agent_lifecycle_journal_create
    RENAME CONSTRAINT ai_agent_lifecycle_journal_create_creation_site_type TO ai_agent_lifecycle_journal_create_origin_type;
ALTER TABLE ai_agent_lifecycle_journal_create RENAME COLUMN creation_site_id TO origin_id;
ALTER TABLE ai_agent_lifecycle_journal_create RENAME COLUMN creation_site_type TO origin_type;

COMMENT ON COLUMN ai_agent_lifecycle_journal_create.origin_type IS 'What kind of thing the AI agent was first embodied in. A pair with origin_id, because the thing can be of more than one kind and no single table holds them all.';

ALTER INDEX ai_agent_ledger_creation_site_idx RENAME TO ai_agent_ledger_origin_idx;
ALTER TABLE ai_agent_ledger
    RENAME CONSTRAINT ai_agent_ledger_creation_site_type TO ai_agent_ledger_origin_type;
ALTER TABLE ai_agent_ledger RENAME COLUMN creation_site_id TO origin_id;
ALTER TABLE ai_agent_ledger RENAME COLUMN creation_site_type TO origin_type;

COMMENT ON COLUMN ai_agent_ledger.origin_type IS 'What kind of thing this AI agent was first embodied in, folded from its creation entry. Not the current embodiment: an AI agent that moved would keep the origin it was created in, and nothing moves one today.';

COMMENT ON INDEX ai_agent_ledger_origin_idx IS 'For asking which AI agents were created in a given workspace or chat, which is a forensic question rather than one live operation asks.';
