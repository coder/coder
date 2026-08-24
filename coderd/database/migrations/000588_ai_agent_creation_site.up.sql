-- Creation site replaces origin, the ledger gains a creation time, and the two
-- containers that hold an AI agent gain an occupancy count.
--
-- Origin is the identity code's word for the pair naming what an agent was
-- created in. Creation site is ours, defined under Terminology in
-- poc_audit/entity_model.md. The corpus uses "origin" in a more general sense,
-- including for whether a fact is institutional or material, so keeping the
-- schema's word would collide with it.

ALTER TABLE ai_agent_ledger RENAME COLUMN origin_type TO creation_site_type;
ALTER TABLE ai_agent_ledger RENAME COLUMN origin_id TO creation_site_id;
ALTER TABLE ai_agent_ledger
    RENAME CONSTRAINT ai_agent_ledger_origin_type TO ai_agent_ledger_creation_site_type;
ALTER INDEX ai_agent_ledger_origin_idx RENAME TO ai_agent_ledger_creation_site_idx;

COMMENT ON COLUMN ai_agent_ledger.creation_site_type IS 'What kind of thing this AI agent was created in, folded from its creation entry. Not where the agent now is: an agent that moved would keep the site it was created in, and nothing moves one today.';

COMMENT ON INDEX ai_agent_ledger_creation_site_idx IS 'For asking which AI agents were created in a given workspace or chat tree, which is a forensic question rather than one live operation asks.';

ALTER TABLE ai_agent_lifecycle_journal_create RENAME COLUMN origin_type TO creation_site_type;
ALTER TABLE ai_agent_lifecycle_journal_create RENAME COLUMN origin_id TO creation_site_id;
ALTER TABLE ai_agent_lifecycle_journal_create
    RENAME CONSTRAINT ai_agent_lifecycle_journal_create_origin_type TO ai_agent_lifecycle_journal_create_creation_site_type;

COMMENT ON COLUMN ai_agent_lifecycle_journal_create.creation_site_type IS 'What kind of thing the AI agent was created in. A pair with creation_site_id, because the thing can be of more than one kind and no single table holds them all.';

-- The ledger gains the creation time, folded from the creation entry's
-- effective date. Nothing is added to the journal: the effective date of a
-- creation is when the agent came into being, and a second column beside it
-- would record one fact twice.
--
-- This reverses one of the three absences the table comment claimed. The
-- reason given there, that a second copy could disagree with the first, argues
-- equally against every other column here, all of which are folds. Folding one
-- more is consistent; the original absence was not.
ALTER TABLE ai_agent_ledger ADD COLUMN creation_time timestamptz;

UPDATE ai_agent_ledger AS l
SET creation_time = j.effective_date
FROM ai_agent_lifecycle_journal AS j
WHERE j.subject = l.id
  AND j.event = 'create';

-- Every AI agent the identity code created gets a ledger row under its own
-- identifier, so that the referents below resolve without a decision about
-- which identifier space they live in.
--
-- The actor is the owner, which is what entity.CreateAIAgent records for a
-- creation it performs itself, so this attributes the entry the same way rather
-- than inventing an attribution for it. The effective date is when the agent
-- was created and the recording date is now, which is the ordinary shape of an
-- entry made later than the event it records.
--
-- Deleted agents are left out, there being no actor to attribute their
-- retirement to. The system actor exists but is superseded and nothing new is
-- to use it, and naming the owner would assert the owner ended an agent the
-- orphan sweep ended. A referent still pointing at a deleted agent therefore
-- fails the foreign keys below, loudly, rather than being papered over here.
WITH legacy AS (
    SELECT
        a.user_id,
        a.owner_user_id,
        a.origin_type::text AS creation_site_type,
        a.origin_id AS creation_site_id,
        a.created_at
    FROM ai_agents AS a
    WHERE NOT a.deleted
      AND NOT EXISTS (SELECT 1 FROM ai_agent_ledger AS l WHERE l.id = a.user_id)
),
entry AS (
    INSERT INTO ai_agent_lifecycle_journal
        (entry_id, recording_date, effective_date, actor_type, actor, event, subject)
    SELECT
        nextval('ai_agent_lifecycle_journal_entry_seq'),
        now(),
        legacy.created_at,
        'user',
        legacy.owner_user_id,
        'create',
        legacy.user_id
    FROM legacy
    RETURNING entry_id, subject, effective_date
),
line AS (
    INSERT INTO ai_agent_lifecycle_journal_create
        (entry_id, line, creation_site_type, creation_site_id)
    SELECT entry.entry_id, 0, legacy.creation_site_type, legacy.creation_site_id
    FROM entry
    JOIN legacy ON legacy.user_id = entry.subject
    RETURNING entry_id
)
INSERT INTO ai_agent_ledger
    (id, owner_type, owner_id, state, posting_reference,
     creation_site_type, creation_site_id, creation_time)
SELECT
    entry.subject,
    'user',
    legacy.owner_user_id,
    'active',
    entry.entry_id,
    legacy.creation_site_type,
    legacy.creation_site_id,
    entry.effective_date
FROM entry
JOIN legacy ON legacy.user_id = entry.subject;

ALTER TABLE ai_agent_ledger ALTER COLUMN creation_time SET NOT NULL;

COMMENT ON COLUMN ai_agent_ledger.creation_time IS 'When this AI agent came into being, folded from the effective date of its creation entry.';

COMMENT ON TABLE ai_agent_ledger IS 'Current state of each AI agent identity. Two absences are deliberate. There is no workspace or sandbox reference, because an AI agent''s identity is independent of where it runs and may outlive any particular sandbox. There is no execution state, because an identity and a run of it are different things, and a schema merging them forecloses reconstituting an AI agent from a previous session.';

-- Occupancy. How many agents a container holds is a fact about the container,
-- per "Capacity belongs to the container" in poc_audit/entity_model.md, and
-- replaces idx_ai_agents_origin, which states the same limit as a uniqueness
-- rule over agents.
--
-- The container is the chat tree, not the chat. A sub-chat resolves to its
-- root's agent, so one agent has always served a whole tree. The tree is an
-- entity with no data structure of its own, and the root chat's identifier
-- stands in for an identifier the tree does not have, which is why data about
-- the tree lives in the root chat's row. This comment can migrate to corpus.
--
-- Enforcement is a conditional update aimed at the root, incrementing where the
-- count is zero, with no rows affected meaning occupied. The CHECK is a
-- backstop against a caller that failed to resolve to the root, which is a bug
-- rather than a legitimate posting.
ALTER TABLE chats
    ADD COLUMN occupancy_count integer NOT NULL DEFAULT 0,
    ADD CONSTRAINT chats_occupancy_only_on_root_chats
        CHECK (root_chat_id IS NULL OR occupancy_count = 0);

UPDATE chats AS c
SET occupancy_count = 1
WHERE c.root_chat_id IS NULL
  AND EXISTS (
      SELECT 1 FROM ai_agents AS a
      WHERE NOT a.deleted
        AND a.origin_type = 'chat'
        AND a.origin_id = c.id
  );

COMMENT ON COLUMN chats.occupancy_count IS 'How many live AI agents this chat tree holds, on the root chat because the tree has no row of its own. Zero on a non-root chat by constraint: the count is meaningless there and a value would read as a second tree.';

-- Refresh chats_expanded to carry the new column. The gentest
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
    c.occupancy_count
   FROM ((chats c
     LEFT JOIN chats root ON ((root.id = COALESCE(c.root_chat_id, c.parent_chat_id))))
     JOIN visible_users owner ON ((owner.id = c.owner_id)));

-- A sandbox is a container of the same kind, without the tree, so one row is
-- one sandbox and the conditional update aims at the sandbox itself.
--
-- Here the count records rather than enforces. A sandbox's occupant is a single
-- column set at insert, so a second occupant is structurally impossible and a
-- ceiling protects nothing. What the count adds is vacating, which has no
-- representation today: soft deleting a sandbox empties it and nothing says so.
-- A soft deleted sandbox is gone; an unoccupied sandbox is empty. They coincide
-- only because nothing empties a sandbox without deleting it, and recording
-- both says which is which before something does.
--
-- No CHECK, deliberately. The conditional update is the mechanism, and a
-- backstop buys little on a table expected to become a ledger shortly.
ALTER TABLE ai_sandboxes
    ADD COLUMN occupancy_count integer NOT NULL DEFAULT 0;

UPDATE ai_sandboxes SET occupancy_count = 1 WHERE NOT deleted;

COMMENT ON COLUMN ai_sandboxes.occupancy_count IS 'How many AI agents this sandbox holds. Distinct from deleted: a soft deleted sandbox is gone and an unoccupied one is empty, and today they coincide because nothing empties a sandbox without deleting it.';

-- The referents come to hold a ledger identifier. They already do, the two
-- identifier spaces being one after the backfill above, so this restates where
-- the value comes from rather than moving any data.
ALTER TABLE ai_sandboxes
    DROP CONSTRAINT ai_sandboxes_ai_agent_id_fkey,
    ADD CONSTRAINT ai_sandboxes_ai_agent_id_fkey
        FOREIGN KEY (ai_agent_id) REFERENCES ai_agent_ledger (id);

ALTER TABLE workspace_agents
    DROP CONSTRAINT workspace_agents_ai_agent_id_fkey,
    ADD CONSTRAINT workspace_agents_ai_agent_id_fkey
        FOREIGN KEY (ai_agent_id) REFERENCES ai_agent_ledger (id);

ALTER TABLE workspaces
    DROP CONSTRAINT workspaces_ai_agent_id_fkey,
    ADD CONSTRAINT workspaces_ai_agent_id_fkey
        FOREIGN KEY (ai_agent_id) REFERENCES ai_agent_ledger (id);

-- ai_sandbox_sessions.ai_agent_id is the fourth referent and carries no foreign
-- key, being a snapshot retained after the identity it names is gone. It holds
-- a ledger identifier now for the same reason as the others, and keeps its
-- freedom to outlive the row.
COMMENT ON COLUMN ai_sandbox_sessions.ai_agent_id IS 'AI agent identity snapshot. Not a foreign key; retained after the identity is retired or cleaned up.';

-- One live agent per creation site, restated on the containers above.
DROP INDEX idx_ai_agents_origin;
