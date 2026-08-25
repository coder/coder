-- `discharge` becomes recordable: an entailed ending with no actor, naming what
-- entailed it.
--
-- A credential is accessory to something, and discharge is that thing ending
-- while the holder does not. See "How the credential machine is read" in
-- poc_audit/entity_model.md for the grounds.

-- An entailed operation has no actor, per "The actor column is nullable, and
-- null there means there was no actor" in poc_audit/implementation_patterns.md.
-- The null stands exactly where a normalized form would have had no row.
ALTER TABLE credential_lifecycle_journal
    ALTER COLUMN actor_type DROP NOT NULL,
    ALTER COLUMN actor DROP NOT NULL;

-- What entailed the operation, in one of two forms.
--
-- **Two fields because the entailing thing is not always an entity yet.** The
-- reference names an entry, which is what the model asks for. The annotation
-- says in words what ended, and is for the case where the thing that ended
-- keeps no journal to point at. A sandbox and a workspace are both such cases
-- today, so every discharge this schema can currently record uses the
-- annotation.
--
-- The annotation is annotative in the sense of "Annotative fields are named so,
-- and posting never reads them": it is written for a reader and no posting
-- consults it.
ALTER TABLE credential_lifecycle_journal
    ADD COLUMN entailed_by_entry bigint,
    ADD COLUMN entailed_by_annotation text;

COMMENT ON COLUMN credential_lifecycle_journal.entailed_by_entry IS 'The entry this operation followed from, where the thing that entailed it keeps a journal. Exactly one of this and entailed_by_annotation is set on an entailed entry.';

COMMENT ON COLUMN credential_lifecycle_journal.entailed_by_annotation IS 'What entailed this operation, in words, where the thing that entailed it keeps no journal to reference. Annotative: posting never reads it. Replaced by a proper reference once that thing is an entity.';

-- Never both, whatever the event.
ALTER TABLE credential_lifecycle_journal
    ADD CONSTRAINT credential_lifecycle_journal_entailed_by_one_form
        CHECK (NOT (entailed_by_entry IS NOT NULL AND entailed_by_annotation IS NOT NULL));

-- A discharge has no actor and says what entailed it. Both directions are
-- constrained, so a discharge carrying an actor is refused as well as one
-- missing its reference.
--
-- **`lapse` is entailed and is not named here, which is a cheat.** It still
-- passes a fixed system identity, written when a lapse was classed observed,
-- and it names nothing that entailed it. Adding it to these checks is part of
-- the rework recorded on `LapseCredential`, and doing it here would break every
-- retirement. The constraint states what is true today rather than what should
-- be.
ALTER TABLE credential_lifecycle_journal
    ADD CONSTRAINT credential_lifecycle_journal_discharge_has_no_actor
        CHECK ((event = 'discharge') = (actor IS NULL)),
    ADD CONSTRAINT credential_lifecycle_journal_discharge_names_its_cause
        CHECK ((event = 'discharge') = (entailed_by_entry IS NOT NULL OR entailed_by_annotation IS NOT NULL));
