-- Firstly, we need to drop the old index.
DROP INDEX IF EXISTS workspace_agents_auth_token_idx;

-- Secondly, we need to fix any duplicate auth_tokens.
-- We do this by setting the auth_token of the duplicate agent to a new UUID.
-- The chance of actually getting a UUID collision is extremely low, but
-- thanks to the birthday paradox, it's not impossible.
UPDATE
	workspace_agents
SET
	auth_token = gen_random_uuid()
WHERE
	id
IN (
	SELECT wa1.id
	FROM
		workspace_agents wa1
	INNER JOIN
		workspace_agents wa2
	ON
		wa1.id != wa2.id
	AND
		wa1.created_at >= wa2.created_at
	AND
		wa1.auth_token = wa2.auth_token
);

-- Finally, add the new unique index.
CREATE UNIQUE INDEX workspace_agents_auth_token_uniq_idx ON workspace_agents USING btree (auth_token);
