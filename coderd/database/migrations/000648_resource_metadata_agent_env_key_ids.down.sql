-- Encrypted values are unreadable without the key_id columns, so refuse to
-- drop them while any row is still encrypted. Run `coder server dbcrypt
-- decrypt` (or `delete`) first.
DO $$
BEGIN
IF EXISTS (
	SELECT *
		FROM workspace_resource_metadata
		WHERE value_key_id IS NOT NULL
	) THEN RAISE EXCEPTION 'Cannot drop value_key_id while workspace_resource_metadata rows are still encrypted. Decrypt the database first.';
END IF;
IF EXISTS (
	SELECT *
		FROM workspace_agents
		WHERE environment_variables_key_id IS NOT NULL
	) THEN RAISE EXCEPTION 'Cannot drop environment_variables_key_id while workspace_agents rows are still encrypted. Decrypt the database first.';
END IF;
END
$$;

ALTER TABLE workspace_resource_metadata
	DROP COLUMN IF EXISTS value_key_id;

ALTER TABLE workspace_agents
	DROP COLUMN IF EXISTS environment_variables_key_id;
