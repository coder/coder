-- The ledger. It records what is currently true of each authorization. Its
-- content is derived: the journal below is the book of original entry, and
-- every column here restates something an entry already said.
CREATE TABLE authorization_lifecycle_ledger (
	id uuid PRIMARY KEY,
	-- The parties, each a (type, identifier) pair. Neither is a foreign key,
	-- because SQL cannot declare one into a union of identity tables, and
	-- neither position is restricted to the one kind that occupies it today.
	principal_type text NOT NULL,
	principal_id uuid NOT NULL,
	agent_type text NOT NULL,
	agent_id uuid NOT NULL,
	-- Always empty, which denotes a universal grant: everything the principal
	-- may do, the agent may do. Reserved for a later authorization language.
	-- The constraint is what makes the reservation real rather than a comment,
	-- and it will fail loudly the first time somebody tries to use the field.
	scope text NOT NULL CONSTRAINT authorization_lifecycle_ledger_scope_reserved CHECK (scope = ''),
	state text NOT NULL CONSTRAINT authorization_lifecycle_ledger_state CHECK (state IN ('active', 'terminated')),
	-- The entry most recently posted to this row. Not a foreign key, and none
	-- is available: the journal's key is (entry_id, line), so entry_id alone
	-- is not unique and cannot be made so, the lines of one entry sharing it.
	posting_reference bigint NOT NULL
);

COMMENT ON TABLE authorization_lifecycle_ledger IS 'Current state of each authorization, an agency relation between a principal and an agent. Derived from authorization_lifecycle_journal, which is the book of original entry. Carries no timestamps: when anything happened is recorded there, and a second copy here could disagree with the first.';

COMMENT ON COLUMN authorization_lifecycle_ledger.scope IS 'Reserved for future use; always empty. An empty scope denotes a universal grant, the grant that restricts nothing and therefore has the shortest description. Authorization is not capacity: what an agent can in fact do is restricted by its sandbox, its gateway, and other technical means, and reconciling capacity against authorization is future work that is trivial while every grant is universal.';

COMMENT ON COLUMN authorization_lifecycle_ledger.posting_reference IS 'Identifies the journal entry most recently posted to this row, after the folio that cross references a paper ledger back to its journal. It gives reconciliation a cheap handle, since an entry newer than this one is an entry not yet posted, and it makes posting safe against a race when the update is conditioned on the value it expects to find.';

CREATE INDEX authorization_lifecycle_ledger_agent_idx ON authorization_lifecycle_ledger (agent_type, agent_id);
CREATE INDEX authorization_lifecycle_ledger_principal_idx ON authorization_lifecycle_ledger (principal_type, principal_id);

-- One nextval per entry, shared by every line of it. Not attached as a column
-- default: a default allocates per row, and the lines of one entry must agree.
CREATE SEQUENCE authorization_lifecycle_journal_entry_seq AS bigint;

CREATE TABLE authorization_lifecycle_journal (
	entry_id bigint NOT NULL,
	line smallint NOT NULL CONSTRAINT authorization_lifecycle_journal_line_non_negative CHECK (line >= 0),

	-- Entry level. Written on line 0 and null on every later line, so that a
	-- value written once cannot disagree with itself. Each column carries its
	-- own check so a violation names the column that caused it.
	recording_date timestamptz DEFAULT now()
		CONSTRAINT authorization_lifecycle_journal_recording_date_on_first_line
		CHECK ((line = 0) = (recording_date IS NOT NULL)),
	effective_date timestamptz DEFAULT now()
		CONSTRAINT authorization_lifecycle_journal_effective_date_on_first_line
		CHECK ((line = 0) = (effective_date IS NOT NULL)),
	actor_type text
		CONSTRAINT authorization_lifecycle_journal_actor_type_on_first_line
		CHECK ((line = 0) = (actor_type IS NOT NULL)),
	actor uuid
		CONSTRAINT authorization_lifecycle_journal_actor_on_first_line
		CHECK ((line = 0) = (actor IS NOT NULL)),

	-- Line level. Present on every line.
	event text NOT NULL,
	-- Self reference: every subject of this journal is an authorization, so
	-- the table's name carries what a type column would. No foreign key yet;
	-- see the open question in poc_audit/implementation_patterns.md.
	subject uuid NOT NULL,

	PRIMARY KEY (entry_id, line)
);

COMMENT ON TABLE authorization_lifecycle_journal IS 'Journal of persistent state changes to authorizations, against which the ledger and the world can be reconciled. One journal per entity: sharing one with another entity would assert that their lifecycles are the same shape and will remain so. Distinct from audit_logs, which is a separate mechanism recording requests.';

COMMENT ON COLUMN authorization_lifecycle_journal.entry_id IS 'Identifies the entry. An entry may occupy several lines, which share this value and differ by line number. Multiple lines express an atomic group: one event rather than several that coincide, so an entry that ends one authorization and begins another leaves no gap between them.';

COMMENT ON COLUMN authorization_lifecycle_journal.recording_date IS 'When the entry was made, never when the event occurred. Never passed as a parameter: line 0 omits the column and takes the default, later lines write a literal null, so no caller can supply, override, or backdate it.';

COMMENT ON COLUMN authorization_lifecycle_journal.effective_date IS 'When the event occurred, which for an observed transition may be long before it was recorded. Where the time is not known, the caller reads the clock at the earliest moment it can vouch for, making the value an upper bound rather than a measurement. How good the value is cannot be carried by this column.';

CREATE INDEX authorization_lifecycle_journal_subject_idx ON authorization_lifecycle_journal (subject);
CREATE INDEX authorization_lifecycle_journal_actor_idx ON authorization_lifecycle_journal (actor_type, actor) WHERE actor IS NOT NULL;
