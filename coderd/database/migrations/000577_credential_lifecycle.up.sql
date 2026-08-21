-- The ledger. What is currently true of each credential. Derived from the
-- journal below, which is the book of original entry.
CREATE TABLE credential_lifecycle_ledger (
	id uuid PRIMARY KEY,
	-- The party the credential authenticates, as a (type, identifier) pair.
	-- Not a foreign key: SQL cannot declare one into a union of identity
	-- tables, and nothing restricts this position to the one kind of holder
	-- occupying it today.
	holder_type text NOT NULL,
	holder_id uuid NOT NULL,
	-- What kind of credential this is, and the value that kind requires. No
	-- constraint on the type: production would want one, since each type needs
	-- code able to validate it and a type with no such code names a credential
	-- that can never be validated, but that guards a code path rather than the
	-- data and is not needed here.
	credential_type text NOT NULL,
	credential_value text NOT NULL,
	state text NOT NULL CONSTRAINT credential_lifecycle_ledger_state CHECK (state IN ('valid', 'invalid')),
	-- Null means no expiry. That is not a convention: it stands exactly where
	-- a row would have been absent had expirations been kept in a table of
	-- their own, and the two forms are defined as equivalent.
	expires_at timestamptz,
	posting_reference bigint NOT NULL
);

COMMENT ON TABLE credential_lifecycle_ledger IS 'Current state of each credential. A credential is a means of exercising authority and not the authority itself: a grant stands whether or not one has been issued, and the two are reconciled against each other only because neither determines the other. Carries no creation time, the journal recording when.';

COMMENT ON COLUMN credential_lifecycle_ledger.id IS 'Identifies the credential, and is deliberately not derived from its secret. Letting a secret name the credential carrying it would assume every credential is a password, and would put a secret in every reference to one.';

COMMENT ON COLUMN credential_lifecycle_ledger.credential_type IS 'Two types exist in the proof of concept. A password holds the hex of an unsalted SHA-256 digest of the secret, unsalted because the secret is randomly generated and high entropy, and matching what coderd/apikey already does. A null credential always validates and holds an empty value; it exists for fault isolation in tests, would never be issued in production, and its always-validates path is a proof of concept hazard recorded with the other cheats.';

COMMENT ON COLUMN credential_lifecycle_ledger.credential_value IS 'A container whose interpretation belongs to credential_type rather than to this column, which is why it is text. Postgres bytea holds raw binary and suits a column holding exactly one kind of thing, as api_keys.hashed_secret does; here each type chooses its own encoding, and text keeps that choice self describing. The password type stores a SHA-256 digest as lowercase hex. Hex rather than base64 because it has a single canonical form where base64 has several, and because sha256sum and its equivalents speak it, which matters for a value nobody can recover from the secret. The encoding belongs to the type and can change without a migration.';

COMMENT ON COLUMN credential_lifecycle_ledger.expires_at IS 'The latest moment this credential can be valid. It promises nothing about the credential remaining valid until then, revocation being unconditional. Nothing prevents a row sitting in state valid past this time, since the entries recording expiry are written by a sweep that runs on a period; a reader wanting what is presently usable must test both, and must not do so through a view when verifying, which would be a time of check to time of use error.';

COMMENT ON COLUMN credential_lifecycle_ledger.posting_reference IS 'Identifies the journal entry most recently posted to this row, after the folio that cross references a paper ledger back to its journal.';

CREATE INDEX credential_lifecycle_ledger_holder_idx ON credential_lifecycle_ledger (holder_type, holder_id);

-- One nextval per entry, shared by every line of it.
CREATE SEQUENCE credential_lifecycle_journal_entry_seq AS bigint;

CREATE TABLE credential_lifecycle_journal (
	entry_id bigint NOT NULL,
	line smallint NOT NULL CONSTRAINT credential_lifecycle_journal_line_non_negative CHECK (line >= 0),

	-- Entry level. Written on line 0 and null on every later line.
	recording_date timestamptz DEFAULT now()
		CONSTRAINT credential_lifecycle_journal_recording_date_on_first_line
		CHECK ((line = 0) = (recording_date IS NOT NULL)),
	effective_date timestamptz DEFAULT now()
		CONSTRAINT credential_lifecycle_journal_effective_date_on_first_line
		CHECK ((line = 0) = (effective_date IS NOT NULL)),
	actor_type text
		CONSTRAINT credential_lifecycle_journal_actor_type_on_first_line
		CHECK ((line = 0) = (actor_type IS NOT NULL)),
	actor uuid
		CONSTRAINT credential_lifecycle_journal_actor_on_first_line
		CHECK ((line = 0) = (actor IS NOT NULL)),

	-- Line level. Present on every line.
	event text NOT NULL,
	-- Self reference: every subject here is a credential, so the table's name
	-- carries what a type column would.
	subject uuid NOT NULL,

	PRIMARY KEY (entry_id, line)
);

COMMENT ON TABLE credential_lifecycle_journal IS 'Journal of persistent state changes to credentials. One journal per entity: sharing one would assert that two lifecycles are the same shape and will remain so. Distinct from audit_logs, which is a separate mechanism recording requests.';

COMMENT ON COLUMN credential_lifecycle_journal.entry_id IS 'Identifies the entry. An entry may occupy several lines sharing this value, expressing an atomic group: rotation issues one credential and revokes another as a single event, so that no interval passes without a valid one.';

COMMENT ON COLUMN credential_lifecycle_journal.effective_date IS 'When the event occurred. For an expiry this is the expiry time and not the moment a sweep noticed, so an entry written late records the same fact at the same moment. It is the earlier of the event time and the recording time, which keeps it from ever claiming the journal foresaw something.';

CREATE INDEX credential_lifecycle_journal_subject_idx ON credential_lifecycle_journal (subject);
CREATE INDEX credential_lifecycle_journal_actor_idx ON credential_lifecycle_journal (actor_type, actor) WHERE actor IS NOT NULL;
