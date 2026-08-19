CREATE TABLE valid_credentials (
	-- An entity identity is always a (type, identifier) pair. SQL cannot
	-- express a reference into a union of the identity tables, so the type
	-- says which table holds the identifier.
	actor_type text NOT NULL,
	actor uuid NOT NULL,
	password text NOT NULL
);

COMMENT ON TABLE valid_credentials IS 'Credentials that are currently valid. Membership is validity: revoking a credential deletes its row, and the credential journal holds the history including when that happened. There is deliberately no key. An actor may hold more than one valid credential at a time, so that rotation can overlap rather than requiring a moment with no valid credential.';

COMMENT ON COLUMN valid_credentials.password IS 'A plaintext password. This is a proof of concept cheat, enumerated with the others under PoC cheats in poc_audit/work_breakdown.md. The mandates in poc_audit/security_findings.md require that only a non reversible form of a credential be stored, and this violates that deliberately and temporarily.';

-- Lookups are by identifier. The type is carried for correctness rather than
-- selectivity, since a uuid is already unique across every identity table.
CREATE INDEX idx_valid_credentials_actor ON valid_credentials (actor);
