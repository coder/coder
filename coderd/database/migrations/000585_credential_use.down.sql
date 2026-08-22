ALTER TABLE credential_ledger
    DROP COLUMN use_posting_reference,
    DROP COLUMN last_used,
    DROP COLUMN last_presented;

ALTER TABLE credential_ledger
    RENAME COLUMN lifecycle_posting_reference TO posting_reference;

DROP TABLE credential_use_journal;

DROP SEQUENCE credential_use_journal_entry_seq;
