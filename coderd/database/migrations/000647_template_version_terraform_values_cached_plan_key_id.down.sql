-- Encrypted values are unreadable without the key_id column, so refuse to
-- drop it while any row is still encrypted. Run `coder server dbcrypt
-- decrypt` (or `delete`) first.
DO $$
BEGIN
IF EXISTS (
	SELECT *
		FROM template_version_terraform_values
		WHERE cached_plan_key_id IS NOT NULL
	) THEN RAISE EXCEPTION 'Cannot drop cached_plan_key_id while template_version_terraform_values rows are still encrypted. Decrypt the database first.';
END IF;
END
$$;

ALTER TABLE template_version_terraform_values
	DROP COLUMN IF EXISTS cached_plan_key_id;
