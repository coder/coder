-- An entry becomes an atomic group: one party, one moment, several subjects,
-- each with its own event.
--
-- The schema already claimed this. The comment on
-- credential_lifecycle_journal.entry_id says an entry may occupy several lines
-- expressing an atomic group, and the primary key on (entry_id) forbade it.
-- entity_model.md holds that a rotation issues one credential and revokes
-- another as one entry, so that no interval passes without a valid one, and
-- that recording it as two entries would assert the very gap the overlap exists
-- to prevent. The bar against rotation in the proof of concept was lifted on
-- 2026-08-24, so the position now has to be implementable.
--
-- **subject and event move together.** Moving the subject alone looks
-- sufficient and is not: a rotation is an issue and a revoke, so a single event
-- column leaves it inexpressible however many subjects an entry can name.
--
-- **One actor per entry is untouched**, and is a position rather than an
-- artifact of the old shape. See "One actor per entry, not two" in
-- poc_audit/entity_model.md. One subject per entry was never a position.

CREATE TABLE credential_lifecycle_journal_line (
    entry_id bigint NOT NULL REFERENCES credential_lifecycle_journal (entry_id),
    line smallint NOT NULL,
    subject uuid NOT NULL,
    event text NOT NULL,
    PRIMARY KEY (entry_id, line),
    CONSTRAINT credential_lifecycle_journal_line_non_negative CHECK (line >= 0)
);

COMMENT ON TABLE credential_lifecycle_journal_line IS 'Lines of the credential journal, one per credential the entry acts on. The entry says who acted and when; a line says which credential and what happened to it. An entry with two lines is an atomic group: a rotation issues one credential and revokes another as a single event.';

COMMENT ON COLUMN credential_lifecycle_journal_line.line IS 'Subordinate to the entry and starting at zero, as in the denormalized form. Nothing enforces that a type specific line table does not claim a number this table has not issued beyond the foreign key that now points here.';

COMMENT ON COLUMN credential_lifecycle_journal_line.subject IS 'The credential this line acts on. An entry names one party and may name several subjects, which is the asymmetry the atomic group rests on.';

-- Every entry so far has exactly one subject, so each becomes line zero.
INSERT INTO credential_lifecycle_journal_line (entry_id, line, subject, event)
SELECT entry_id, 0, subject, event
FROM credential_lifecycle_journal;

-- **Two checks are dropped rather than reproduced, and that is a loss.**
-- Migration 000590 made a discharge carry no actor and name what entailed it,
-- as table checks over event, actor and the entailing reference. With event on
-- another table a check cannot see both sides, and Postgres has no cross table
-- check.
--
-- Eric, 2026-08-25: drop them and consign the property to reconciliation. The
-- door in coderd/entity/credential.go still writes only what the model permits,
-- so what is lost is enforcement and not correctness. This is the same standing
-- the api_key line table already records for line numbering, "a reconciliation
-- rather than a constraint".
--
-- The remaining check survives untouched, testing only columns that stay:
-- an entailed entry names its cause in one form and never in both.
ALTER TABLE credential_lifecycle_journal
    DROP CONSTRAINT credential_lifecycle_journal_discharge_names_its_cause,
    DROP CONSTRAINT credential_lifecycle_journal_discharge_has_no_actor;

DROP INDEX credential_lifecycle_journal_subject_idx;

ALTER TABLE credential_lifecycle_journal
    DROP COLUMN subject,
    DROP COLUMN event;

CREATE INDEX credential_lifecycle_journal_line_subject_idx
    ON credential_lifecycle_journal_line USING btree (subject);

-- The type specific lines describe a line, not an entry. Pointing at the line
-- is what stops an api_key line claiming a number the entry never issued.
ALTER TABLE credential_lifecycle_journal_api_key
    DROP CONSTRAINT credential_lifecycle_journal_api_key_entry_fkey,
    ADD CONSTRAINT credential_lifecycle_journal_api_key_line_fkey
        FOREIGN KEY (entry_id, line)
        REFERENCES credential_lifecycle_journal_line (entry_id, line);
