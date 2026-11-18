-- Encrypted values are unreadable without the key_id column, so refuse to
-- drop it while any row is still encrypted. Run `coder server dbcrypt
-- decrypt` (or `delete`) first.
DO $$
BEGIN
IF EXISTS (
	SELECT *
		FROM workspace_build_parameters
		WHERE value_key_id IS NOT NULL
	) THEN RAISE EXCEPTION 'Cannot drop value_key_id while workspace_build_parameters rows are still encrypted. Decrypt the database first.';
END IF;
END
$$;

ALTER TABLE workspace_build_parameters
	DROP COLUMN IF EXISTS value_key_id,
	DROP COLUMN IF EXISTS sensitive;
