-- Before dropping this table, we need to check if there exist any
-- foreign key references to it. We do this by checking the following:
-- user_links.oauth_access_token_key_id
-- user_links.oauth_refresh_token_key_id
-- git_auth_links.oauth_access_token_key_id
-- git_auth_links.oauth_refresh_token_key_id
DO $$
BEGIN
IF EXISTS (
	SELECT *
		FROM user_links
		WHERE oauth_access_token_key_id IS NOT NULL
		OR oauth_refresh_token_key_id IS NOT NULL
	) THEN RAISE EXCEPTION 'Cannot drop dbcrypt_keys table as there are still foreign key references to it from user_links.';
END IF;

IF EXISTS (
	SELECT *
		FROM git_auth_links
		WHERE oauth_access_token_key_id IS NOT NULL
		OR oauth_refresh_token_key_id IS NOT NULL
	) THEN RAISE EXCEPTION 'Cannot drop dbcrypt_keys table as there are still foreign key references to it from git_auth_links.';
END IF;

END
$$;


-- Drop the columns first.
ALTER TABLE git_auth_links
	DROP COLUMN IF EXISTS oauth_access_token_key_id,
	DROP COLUMN IF EXISTS oauth_refresh_token_key_id;

ALTER TABLE user_links
	DROP COLUMN IF EXISTS oauth_access_token_key_id,
	DROP COLUMN IF EXISTS oauth_refresh_token_key_id;

-- Finally, drop the table.
DROP TABLE IF EXISTS dbcrypt_keys;
