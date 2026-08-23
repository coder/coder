DROP INDEX IF EXISTS ai_agent_ledger_origin_idx;

ALTER TABLE ai_agent_ledger
    DROP CONSTRAINT ai_agent_ledger_origin_type,
    DROP COLUMN origin_type,
    DROP COLUMN origin_id;

DROP TABLE ai_agent_lifecycle_journal_create;

ALTER TABLE ai_agent_lifecycle_journal
    DROP CONSTRAINT ai_agent_lifecycle_journal_pkey,
    ADD COLUMN line smallint NOT NULL DEFAULT 0;

ALTER TABLE ai_agent_lifecycle_journal
    ALTER COLUMN line DROP DEFAULT,
    ALTER COLUMN recording_date DROP NOT NULL,
    ALTER COLUMN effective_date DROP NOT NULL,
    ALTER COLUMN actor_type DROP NOT NULL,
    ALTER COLUMN actor DROP NOT NULL,
    ADD CONSTRAINT ai_agent_lifecycle_journal_pkey PRIMARY KEY (entry_id, line),
    ADD CONSTRAINT ai_agent_lifecycle_journal_actor_on_first_line CHECK ((line = 0) = (actor IS NOT NULL)),
    ADD CONSTRAINT ai_agent_lifecycle_journal_actor_type_on_first_line CHECK ((line = 0) = (actor_type IS NOT NULL)),
    ADD CONSTRAINT ai_agent_lifecycle_journal_effective_date_on_first_line CHECK ((line = 0) = (effective_date IS NOT NULL)),
    ADD CONSTRAINT ai_agent_lifecycle_journal_line_non_negative CHECK (line >= 0),
    ADD CONSTRAINT ai_agent_lifecycle_journal_recording_date_on_first_line CHECK ((line = 0) = (recording_date IS NOT NULL));
