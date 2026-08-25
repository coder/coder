ALTER TABLE credential_lifecycle_journal
    DROP CONSTRAINT credential_lifecycle_journal_discharge_names_its_cause,
    DROP CONSTRAINT credential_lifecycle_journal_discharge_has_no_actor,
    DROP CONSTRAINT credential_lifecycle_journal_entailed_by_one_form,
    DROP COLUMN entailed_by_annotation,
    DROP COLUMN entailed_by_entry;

-- Discharge entries carry no actor, so restoring NOT NULL would fail on them.
-- They go with the capability that wrote them: after this migration the
-- transition is not expressible, so an entry recording one is not either.
DELETE FROM credential_lifecycle_journal WHERE event = 'discharge';

ALTER TABLE credential_lifecycle_journal
    ALTER COLUMN actor_type SET NOT NULL,
    ALTER COLUMN actor SET NOT NULL;
