-- Drop the unique index
DROP INDEX IF EXISTS workspace_agents_auth_token_uniq_idx;

-- Recreate the old non-unique index.
CREATE INDEX workspace_agents_auth_token_idx ON workspace_agents USING btree (auth_token);
