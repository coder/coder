-- Reverses only while every entry has one line. Two subjects cannot go back
-- into one row, so an entry recording a rotation is lost rather than folded.
-- `tigre` is a proof of concept branch with no deployment to migrate down, so
-- this is written to restore the shape and not to preserve such an entry.

ALTER TABLE credential_lifecycle_journal_api_key
    DROP CONSTRAINT credential_lifecycle_journal_api_key_line_fkey,
    ADD CONSTRAINT credential_lifecycle_journal_api_key_entry_fkey
        FOREIGN KEY (entry_id) REFERENCES credential_lifecycle_journal (entry_id);

DROP INDEX credential_lifecycle_journal_line_subject_idx;

ALTER TABLE credential_lifecycle_journal
    ADD COLUMN subject uuid,
    ADD COLUMN event text;

UPDATE credential_lifecycle_journal AS j
SET subject = l.subject, event = l.event
FROM credential_lifecycle_journal_line AS l
WHERE l.entry_id = j.entry_id AND l.line = 0;

DELETE FROM credential_lifecycle_journal WHERE subject IS NULL;

ALTER TABLE credential_lifecycle_journal
    ALTER COLUMN subject SET NOT NULL,
    ALTER COLUMN event SET NOT NULL;

CREATE INDEX credential_lifecycle_journal_subject_idx
    ON credential_lifecycle_journal USING btree (subject);

ALTER TABLE credential_lifecycle_journal
    ADD CONSTRAINT credential_lifecycle_journal_discharge_has_no_actor
        CHECK ((event = 'discharge') = (actor IS NULL)),
    ADD CONSTRAINT credential_lifecycle_journal_discharge_names_its_cause
        CHECK ((event = 'discharge') = (entailed_by_entry IS NOT NULL OR entailed_by_annotation IS NOT NULL));

DROP TABLE credential_lifecycle_journal_line;
