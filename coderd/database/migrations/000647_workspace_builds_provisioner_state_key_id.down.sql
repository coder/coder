-- Encrypted values are unreadable without the key_id column, so refuse to
-- drop it while any row is still encrypted. Run `coder server dbcrypt
-- decrypt` (or `delete`) first.
DO $$
BEGIN
IF EXISTS (
	SELECT *
		FROM workspace_builds
		WHERE provisioner_state_key_id IS NOT NULL
	) THEN RAISE EXCEPTION 'Cannot drop provisioner_state_key_id while workspace_builds rows are still encrypted. Decrypt the database first.';
END IF;
END
$$;

ALTER TABLE workspace_builds
	DROP COLUMN IF EXISTS provisioner_state_key_id;
