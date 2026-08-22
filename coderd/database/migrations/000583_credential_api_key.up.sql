-- The first heterogeneous credential, and so the first line table.
--
-- An api_key credential carries what a password credential does not: a token
-- name, a set of scopes, and an allow list. That is the heterogeneity the
-- normalized form exists for, per "A journal takes one of two forms, and
-- heterogeneity decides which" in poc_audit/implementation_patterns.md.

-- Ledger side: what an api_key credential currently is.
CREATE TABLE credential_api_key (
    id uuid NOT NULL,
    hashed_secret text NOT NULL,
    token_name text NOT NULL,
    scopes api_key_scope[] NOT NULL,
    allow_list text[] NOT NULL,
    CONSTRAINT credential_api_key_pkey PRIMARY KEY (id),
    CONSTRAINT credential_api_key_id_fkey FOREIGN KEY (id)
        REFERENCES credential_lifecycle_ledger (id) ON DELETE CASCADE,
    CONSTRAINT credential_api_key_allow_list_not_empty
        CHECK ((array_length(allow_list, 1) > 0))
);

COMMENT ON TABLE credential_api_key IS 'What an api_key credential holds beyond what every credential holds. Keyed on the ledger row it belongs to, so the type discriminator on that row says this is the table to read.';

COMMENT ON COLUMN credential_api_key.hashed_secret IS 'Hex of an unsalted SHA-256 digest, as for a password credential. The column is separate rather than shared because a type owns its own state, and two types holding a digest apiece is not one column held in common.';

-- Journal side: what an issuance of an api_key credential carried. The entry
-- table says an issuance occurred, by whom and when; this says with what.
--
-- Only issuance takes parameters. Revocation of an api_key credential carries
-- nothing a revocation of any other credential does not, so it writes no line.
CREATE TABLE credential_lifecycle_journal_api_key (
    entry_id bigint NOT NULL,
    line smallint NOT NULL,
    token_name text NOT NULL,
    scopes api_key_scope[] NOT NULL,
    allow_list text[] NOT NULL,
    CONSTRAINT credential_lifecycle_journal_api_key_pkey PRIMARY KEY (entry_id, line),
    CONSTRAINT credential_lifecycle_journal_api_key_entry_fkey FOREIGN KEY (entry_id)
        REFERENCES credential_lifecycle_journal (entry_id),
    CONSTRAINT credential_lifecycle_journal_api_key_line_non_negative
        CHECK ((line >= 0))
);

COMMENT ON TABLE credential_lifecycle_journal_api_key IS 'Lines of the credential journal describing the issuance of an api_key credential. Line numbers are subordinate to the entry and start at zero, as in the denormalized form. With a second line table nothing would enforce that two of them do not claim the same number within one entry, which is a reconciliation rather than a constraint.';
