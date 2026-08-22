-- The credential's second model: its use. See "The credential use model" in
-- poc_audit/entity_model.md. One journal per model-entity, one ledger per
-- subject-entity, so this adds a journal and posts into the ledger the
-- lifecycle already writes to.

CREATE SEQUENCE credential_use_journal_entry_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

-- The normalized form, entry table only: neither operation takes parameters, so
-- no line table exists and every entry is one line carrying nothing beyond
-- which operation it was.
CREATE TABLE credential_use_journal (
    entry_id bigint NOT NULL,
    recording_date timestamp with time zone DEFAULT now() NOT NULL,
    effective_date timestamp with time zone DEFAULT now() NOT NULL,
    actor_type text NOT NULL,
    actor uuid NOT NULL,
    event text NOT NULL,
    subject uuid NOT NULL,
    annotation_source text,
    CONSTRAINT credential_use_journal_pkey PRIMARY KEY (entry_id),
    CONSTRAINT credential_use_journal_event CHECK ((event = ANY (ARRAY['presentation_accepted'::text, 'presentation_refused'::text])))
);

COMMENT ON TABLE credential_use_journal IS 'Journal of presentations of a credential. Both operations are observed and the actor is the verifier, the party the presentation was made to and so the party that noticed.';

COMMENT ON COLUMN credential_use_journal.annotation_source IS 'Where the presentation arrived from, as the verifier observed it. Reliable, being what the verifier knows about itself, and an annotation because it bears on nothing the operation assigns. There is deliberately no column for who the presenter claimed to be: the declaration of which credential is being presented is the only claim a presentation carries, and it is the subject.';

CREATE INDEX credential_use_journal_subject_idx ON credential_use_journal USING btree (subject);

-- The use model's values, on the ledger row its subject already has. The
-- posting reference follows the journal rather than the subject, so the row
-- carries one per model and the lifecycle's takes its model's name.
ALTER TABLE credential_ledger
    RENAME COLUMN posting_reference TO lifecycle_posting_reference;

ALTER TABLE credential_ledger
    ADD COLUMN last_presented timestamp with time zone,
    ADD COLUMN last_used timestamp with time zone,
    ADD COLUMN use_posting_reference bigint;

COMMENT ON COLUMN credential_ledger.last_presented IS 'When the credential was last offered, however it went. Null is the initial value every variable has, and means it has never been offered.';

COMMENT ON COLUMN credential_ledger.last_used IS 'When the credential was last offered and accepted. Null means never.';

COMMENT ON COLUMN credential_ledger.use_posting_reference IS 'Which entry of the use journal last posted here. Separate from the lifecycle reference because a posting reference follows its journal, which also keeps a posting under one model from making a posting under another lose a race it is not in.';
