-- The AI agent journal takes the normalized form, and the AI agent gains the
-- origin it was created in.
--
-- The two arrive together because the first forces the second. Recording an
-- origin makes `create` an operation with particulars where the others have
-- none, which is the heterogeneity that decides the form, per "A journal takes
-- one of two forms" in poc_audit/implementation_patterns.md. More is coming:
-- `transfer` carries a new owner, a different shape again.

-- The journal becomes an entry table. The line-zero constraints go with the
-- line column, an entry table holding entry level values once by construction,
-- which is what those constraints were emulating.
ALTER TABLE ai_agent_lifecycle_journal
    DROP CONSTRAINT ai_agent_lifecycle_journal_pkey,
    DROP CONSTRAINT ai_agent_lifecycle_journal_actor_on_first_line,
    DROP CONSTRAINT ai_agent_lifecycle_journal_actor_type_on_first_line,
    DROP CONSTRAINT ai_agent_lifecycle_journal_effective_date_on_first_line,
    DROP CONSTRAINT ai_agent_lifecycle_journal_line_non_negative,
    DROP CONSTRAINT ai_agent_lifecycle_journal_recording_date_on_first_line,
    DROP COLUMN line;

ALTER TABLE ai_agent_lifecycle_journal
    ALTER COLUMN recording_date SET NOT NULL,
    ALTER COLUMN effective_date SET NOT NULL,
    ALTER COLUMN actor_type SET NOT NULL,
    ALTER COLUMN actor SET NOT NULL,
    ADD CONSTRAINT ai_agent_lifecycle_journal_pkey PRIMARY KEY (entry_id);

COMMENT ON TABLE ai_agent_lifecycle_journal IS 'Journal of persistent state changes to AI agents, in the normalized form: this is the entry table. Line tables join to it per shape of operation. One journal per entity: sharing one would assert that two lifecycles are the same shape and will remain so. Distinct from audit_logs, which is a separate mechanism recording requests.';

-- What a creation carried. The first line table of this journal, and the only
-- operation with particulars until `transfer` lands.
CREATE TABLE ai_agent_lifecycle_journal_create (
    entry_id bigint NOT NULL,
    line smallint NOT NULL,
    origin_type text NOT NULL,
    origin_id uuid NOT NULL,
    CONSTRAINT ai_agent_lifecycle_journal_create_pkey PRIMARY KEY (entry_id, line),
    CONSTRAINT ai_agent_lifecycle_journal_create_entry_id_fkey FOREIGN KEY (entry_id)
        REFERENCES ai_agent_lifecycle_journal (entry_id) ON DELETE CASCADE,
    CONSTRAINT ai_agent_lifecycle_journal_create_line_non_negative CHECK (line >= 0),
    CONSTRAINT ai_agent_lifecycle_journal_create_origin_type CHECK (origin_type = ANY (ARRAY['chat'::text, 'workspace'::text]))
);

COMMENT ON TABLE ai_agent_lifecycle_journal_create IS 'What a creation of an AI agent carried. A line table of the AI agent journal, joined by entry identifier.';

COMMENT ON COLUMN ai_agent_lifecycle_journal_create.origin_type IS 'What kind of thing the AI agent was first embodied in. A pair with origin_id, because the thing can be of more than one kind and no single table holds them all.';

-- The ledger holds the fold. Origin is set once and no later entry changes it,
-- so the fold is a constant, which is a ledger column like any other.
-- No backfill, deliberately. An AI agent created before origins were recorded
-- has no origin to recover, here or anywhere: nothing on this side held one and
-- the ledger's identifiers relate to nothing that did. Setting NOT NULL without
-- an UPDATE therefore fails on a populated table, with a clear message, rather
-- than inventing a value a later reader would take for a fact.
ALTER TABLE ai_agent_ledger
    ADD COLUMN origin_type text,
    ADD COLUMN origin_id uuid;

ALTER TABLE ai_agent_ledger
    ALTER COLUMN origin_type SET NOT NULL,
    ALTER COLUMN origin_id SET NOT NULL,
    ADD CONSTRAINT ai_agent_ledger_origin_type CHECK (origin_type = ANY (ARRAY['chat'::text, 'workspace'::text]));

COMMENT ON COLUMN ai_agent_ledger.origin_type IS 'What kind of thing this AI agent was first embodied in, folded from its creation entry. Not the current embodiment: an AI agent that moved would keep the origin it was created in, and nothing moves one today.';

CREATE INDEX ai_agent_ledger_origin_idx ON ai_agent_ledger (origin_type, origin_id);

COMMENT ON INDEX ai_agent_ledger_origin_idx IS 'For asking which AI agents were created in a given workspace or chat, which is a forensic question rather than one live operation asks.';
