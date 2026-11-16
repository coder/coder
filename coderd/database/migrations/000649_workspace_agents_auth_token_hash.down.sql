-- Encrypted tokens are unreadable without the key_id column, so refuse to
-- revert while any row is still encrypted. Run `coder server dbcrypt
-- decrypt` (or `delete`) first.
DO $$
BEGIN
IF EXISTS (
	SELECT *
		FROM workspace_agents
		WHERE auth_token_key_id IS NOT NULL
	) THEN RAISE EXCEPTION 'Cannot restore the uuid auth_token column while workspace_agents rows are still encrypted. Decrypt the database first.';
END IF;
END
$$;

-- `dbcrypt delete` blanks lost tokens; a uuid column cannot hold them, so
-- give those agents a fresh random token (they are unusable either way).
UPDATE workspace_agents
	SET auth_token = gen_random_uuid()::text
	WHERE auth_token = '';

ALTER TABLE workspace_agents
	ALTER COLUMN auth_token TYPE uuid USING auth_token::uuid;

COMMENT ON COLUMN workspace_agents.auth_token IS NULL;

ALTER TABLE workspace_agents
	DROP COLUMN IF EXISTS auth_token_key_id;

ALTER TABLE workspace_agents
	DROP COLUMN IF EXISTS auth_token_hashed;

CREATE INDEX workspace_agents_auth_token_idx ON workspace_agents USING btree (auth_token);
