-- The credential journal and ledger take the normalized form, per "A journal
-- takes one of two forms, and heterogeneity decides which" in
-- poc_audit/implementation_patterns.md. Heterogeneity is coming: an api_key
-- credential carries scopes, an allow list and a token name where a password
-- carries a digest, and one table holding both would be columns meaningful for
-- some rows and null for the rest.

-- The journal becomes an entry table. No operation on a credential presently
-- takes parameters, so no line table exists yet and every entry is one line
-- carrying nothing but which operation it was. The line-zero constraints go
-- with the line column: an entry table holds entry level values once, which is
-- what those constraints were emulating.
ALTER TABLE credential_lifecycle_journal
    DROP CONSTRAINT credential_lifecycle_journal_pkey,
    DROP CONSTRAINT credential_lifecycle_journal_actor_on_first_line,
    DROP CONSTRAINT credential_lifecycle_journal_actor_type_on_first_line,
    DROP CONSTRAINT credential_lifecycle_journal_effective_date_on_first_line,
    DROP CONSTRAINT credential_lifecycle_journal_line_non_negative,
    DROP CONSTRAINT credential_lifecycle_journal_recording_date_on_first_line,
    DROP COLUMN line;

ALTER TABLE credential_lifecycle_journal
    ALTER COLUMN recording_date SET NOT NULL,
    ALTER COLUMN effective_date SET NOT NULL,
    ALTER COLUMN actor_type SET NOT NULL,
    ALTER COLUMN actor SET NOT NULL,
    ADD CONSTRAINT credential_lifecycle_journal_pkey PRIMARY KEY (entry_id);

COMMENT ON TABLE credential_lifecycle_journal IS 'Journal of persistent state changes to credentials, in the normalized form: this is the entry table. Line tables join to it as credential operations acquire parameters. One journal per entity: sharing one would assert that two lifecycles are the same shape and will remain so. Distinct from audit_logs, which is a separate mechanism recording requests.';

-- The ledger keeps what is generic to a credential and gives up what is not.
-- credential_value held a digest for one type and the empty string for another,
-- which is two meanings in one column before a third type exists.
CREATE TABLE credential_password (
    id uuid NOT NULL,
    hashed_authenticator text NOT NULL,
    CONSTRAINT credential_password_pkey PRIMARY KEY (id),
    CONSTRAINT credential_password_id_fkey FOREIGN KEY (id)
        REFERENCES credential_lifecycle_ledger (id) ON DELETE CASCADE
);

COMMENT ON TABLE credential_password IS 'What a password credential holds beyond what every credential holds. Keyed on the ledger row it belongs to, which is why this needs no foreign key into a union: the ledger mints the identifier and the type says which table to look in.';

COMMENT ON COLUMN credential_password.hashed_authenticator IS 'Hex of an unsalted SHA-256 digest. No salt is needed for a randomly generated high entropy secret, which is the reasoning coderd/apikey follows.';

INSERT INTO credential_password (id, hashed_authenticator)
SELECT id, credential_value
FROM credential_lifecycle_ledger
WHERE credential_type = 'password';

ALTER TABLE credential_lifecycle_ledger DROP COLUMN credential_value;
