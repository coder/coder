-- Agent auth tokens were stored as plaintext uuids and looked up directly by
-- value. To allow the token to be encrypted at rest (SEC-1491), the column
-- becomes text (ciphertext is not a valid uuid) and authentication switches
-- to a deterministic SHA-256 hash of the canonical uuid string, since
-- dbcrypt ciphertext is non-deterministic and cannot be used in a WHERE
-- clause.
ALTER TABLE workspace_agents
	ALTER COLUMN auth_token TYPE text USING auth_token::text;

COMMENT ON COLUMN workspace_agents.auth_token IS 'Session token the agent presents to authenticate. Stored as the canonical uuid string, or as ciphertext when auth_token_key_id is set (SEC-1491).';

ALTER TABLE workspace_agents
	ADD COLUMN IF NOT EXISTS auth_token_key_id text REFERENCES dbcrypt_keys (active_key_digest);

COMMENT ON COLUMN workspace_agents.auth_token_key_id IS 'The ID of the key used to encrypt auth_token. If this is NULL, auth_token is not encrypted.';

ALTER TABLE workspace_agents
	ADD COLUMN IF NOT EXISTS auth_token_hashed bytea;

-- All pre-existing rows hold plaintext canonical uuid strings: dbcrypt never
-- covered this column, and uuid::text always renders lowercase canonical
-- form, which is exactly what the Go side hashes.
UPDATE workspace_agents
	SET auth_token_hashed = sha256(convert_to(auth_token, 'UTF8'));

ALTER TABLE workspace_agents
	ALTER COLUMN auth_token_hashed SET NOT NULL;

COMMENT ON COLUMN workspace_agents.auth_token_hashed IS 'SHA-256 of the canonical uuid string form of auth_token. Used for authentication lookups so the token itself can be encrypted at rest (SEC-1491).';

-- The lookup moves to the hash column. The index stays non-unique: a
-- soft-deleted prior-build agent can legitimately share a token with the
-- agent of the current build (prebuilds reuse tokens across builds).
DROP INDEX IF EXISTS workspace_agents_auth_token_idx;
CREATE INDEX workspace_agents_auth_token_hashed_idx ON workspace_agents USING btree (auth_token_hashed);
