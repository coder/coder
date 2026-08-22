-- The public half of an api key token. A token is "<key_id>-<secret>", and a
-- verifier finds the credential by the first half before comparing the second.
-- The ledger has to record what it minted, or nothing connects a credential to
-- the api_keys row that mirrors it.
--
-- Added with a default so the statement succeeds on an empty table, then
-- dropped, because a credential minted before key ids existed has no key id to
-- backfill. The unique index below fails loudly rather than inventing one.
ALTER TABLE credential_api_key
    ADD COLUMN key_id text NOT NULL DEFAULT '';

ALTER TABLE credential_api_key
    ALTER COLUMN key_id DROP DEFAULT;

CREATE UNIQUE INDEX credential_api_key_key_id_idx
    ON credential_api_key (key_id);

COMMENT ON COLUMN credential_api_key.key_id IS
    'The public half of the token, and the id of the api_keys row mirroring this credential.';
