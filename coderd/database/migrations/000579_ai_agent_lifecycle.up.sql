-- The ledger. What is currently true of each AI agent. Derived from the
-- journal below, which is the book of original entry.
CREATE TABLE ai_agent_lifecycle_ledger (
	id uuid PRIMARY KEY,
	-- The principal the AI agent belongs to, as a (type, identifier) pair.
	-- Not a foreign key: nothing restricts this position to the one kind of
	-- principal occupying it today, and SQL cannot key into a union of
	-- identity tables anyway.
	--
	-- Ownership is not authorization. An AI agent whose every grant has been
	-- revoked still belongs to somebody, so this survives what the
	-- authorization ledger records and is not a copy of it.
	owner_type text NOT NULL,
	owner_id uuid NOT NULL,
	state text NOT NULL CONSTRAINT ai_agent_lifecycle_ledger_state
		CHECK (state IN ('active', 'dormant', 'retired')),
	posting_reference bigint NOT NULL
);

COMMENT ON TABLE ai_agent_lifecycle_ledger IS 'Current state of each AI agent identity. Three absences are deliberate. There is no workspace or sandbox reference, because an AI agent''s identity is independent of where it runs and may outlive any particular sandbox. There is no execution state, because an identity and a run of it are different things, and a schema merging them forecloses reconstituting an AI agent from a previous session. There is no creation time, because the journal records when this row came to exist and a second copy could disagree with the first.';

COMMENT ON COLUMN ai_agent_lifecycle_ledger.state IS 'dormant is reserved for future use and is unreachable in the machine the proof of concept implements, which has active and retired only. It is in the enum now so that supporting reconstitution later costs no migration, which means code switching exhaustively over these values must handle a state that cannot occur.';

CREATE INDEX ai_agent_lifecycle_ledger_owner_idx ON ai_agent_lifecycle_ledger (owner_type, owner_id);

-- One nextval per entry, shared by every line of it.
CREATE SEQUENCE ai_agent_lifecycle_journal_entry_seq AS bigint;

CREATE TABLE ai_agent_lifecycle_journal (
	entry_id bigint NOT NULL,
	line smallint NOT NULL CONSTRAINT ai_agent_lifecycle_journal_line_non_negative CHECK (line >= 0),

	-- Entry level. Written on line 0 and null on every later line.
	recording_date timestamptz DEFAULT now()
		CONSTRAINT ai_agent_lifecycle_journal_recording_date_on_first_line
		CHECK ((line = 0) = (recording_date IS NOT NULL)),
	effective_date timestamptz DEFAULT now()
		CONSTRAINT ai_agent_lifecycle_journal_effective_date_on_first_line
		CHECK ((line = 0) = (effective_date IS NOT NULL)),
	actor_type text
		CONSTRAINT ai_agent_lifecycle_journal_actor_type_on_first_line
		CHECK ((line = 0) = (actor_type IS NOT NULL)),
	actor uuid
		CONSTRAINT ai_agent_lifecycle_journal_actor_on_first_line
		CHECK ((line = 0) = (actor IS NOT NULL)),

	-- Line level. Present on every line.
	event text NOT NULL,
	-- Self reference: every subject here is an AI agent, so the table's name
	-- carries what a type column would.
	subject uuid NOT NULL,

	PRIMARY KEY (entry_id, line)
);

COMMENT ON TABLE ai_agent_lifecycle_journal IS 'Journal of persistent state changes to AI agent identities. One journal per entity: sharing one would assert that two lifecycles are the same shape and will remain so. Distinct from audit_logs, which is a separate mechanism recording requests.';

COMMENT ON COLUMN ai_agent_lifecycle_journal.effective_date IS 'When the event occurred, which for an observed transition may be long before it was recorded. A process that finished on a Tuesday and was noticed on a Friday has its finish recorded on the Friday and dated the Tuesday. It is the earlier of the event time and the recording time, which keeps it from ever claiming the journal foresaw something.';

CREATE INDEX ai_agent_lifecycle_journal_subject_idx ON ai_agent_lifecycle_journal (subject);
CREATE INDEX ai_agent_lifecycle_journal_actor_idx ON ai_agent_lifecycle_journal (actor_type, actor) WHERE actor IS NOT NULL;
