CREATE TABLE entity_journal (
	-- BIGSERIAL because entries require distinct identifiers and timestamps do
	-- not provide them. Entries written in one transaction share a time.
	id BIGSERIAL PRIMARY KEY,
	recorded_at timestamptz NOT NULL,
	event text NOT NULL,
	-- The type names which table holds the primary key the identifier refers
	-- to. It stands in for a foreign key into a union of the identity tables,
	-- which SQL cannot express.
	subject_type text NOT NULL,
	subject uuid NOT NULL,
	actor_type text NOT NULL,
	actor uuid NOT NULL
);

COMMENT ON TABLE entity_journal IS 'Journal of persistent state changes to entities, against which the state of the world can be reconciled. Distinct from audit_logs, which is a separate mechanism recording requests.';
