-- Encrypted values are unreadable without the key_id column, so refuse to
-- drop it while any row is still encrypted. Run `coder server dbcrypt
-- decrypt` (or `delete`) first.
DO $$
BEGIN
IF EXISTS (
	SELECT *
		FROM external_auth_links
		WHERE oauth_extra_key_id IS NOT NULL
	) THEN RAISE EXCEPTION 'Cannot drop oauth_extra_key_id while external_auth_links rows are still encrypted. Decrypt the database first.';
END IF;
END
$$;

ALTER TABLE external_auth_links
	DROP COLUMN IF EXISTS oauth_extra_key_id;
