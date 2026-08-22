ALTER TABLE credential_lifecycle_ledger
    ADD COLUMN credential_value text NOT NULL DEFAULT '';

UPDATE credential_lifecycle_ledger l
SET credential_value = p.hashed_authenticator
FROM credential_password p
WHERE p.id = l.id;

ALTER TABLE credential_lifecycle_ledger ALTER COLUMN credential_value DROP DEFAULT;

DROP TABLE credential_password;

ALTER TABLE credential_lifecycle_journal
    DROP CONSTRAINT credential_lifecycle_journal_pkey,
    ALTER COLUMN recording_date DROP NOT NULL,
    ALTER COLUMN effective_date DROP NOT NULL,
    ALTER COLUMN actor_type DROP NOT NULL,
    ALTER COLUMN actor DROP NOT NULL,
    ADD COLUMN line smallint NOT NULL DEFAULT 0;

ALTER TABLE credential_lifecycle_journal ALTER COLUMN line DROP DEFAULT;

ALTER TABLE credential_lifecycle_journal
    ADD CONSTRAINT credential_lifecycle_journal_pkey PRIMARY KEY (entry_id, line),
    ADD CONSTRAINT credential_lifecycle_journal_actor_on_first_line CHECK (((line = 0) = (actor IS NOT NULL))),
    ADD CONSTRAINT credential_lifecycle_journal_actor_type_on_first_line CHECK (((line = 0) = (actor_type IS NOT NULL))),
    ADD CONSTRAINT credential_lifecycle_journal_effective_date_on_first_line CHECK (((line = 0) = (effective_date IS NOT NULL))),
    ADD CONSTRAINT credential_lifecycle_journal_line_non_negative CHECK ((line >= 0)),
    ADD CONSTRAINT credential_lifecycle_journal_recording_date_on_first_line CHECK (((line = 0) = (recording_date IS NOT NULL)));
