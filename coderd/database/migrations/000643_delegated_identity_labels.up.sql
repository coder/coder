ALTER TABLE delegated_identities ADD COLUMN labels jsonb NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN delegated_identities.labels IS 'What whoever created the identity said it is for, as a flat map of strings, for example the session it serves. Set at creation and never changed. Coder does not verify it, so nothing is decided by it.';
