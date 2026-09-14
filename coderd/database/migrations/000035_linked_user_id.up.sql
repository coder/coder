CREATE TABLE IF NOT EXISTS user_links (
	user_id uuid NOT NULL,
	login_type login_type NOT NULL,
	linked_id text DEFAULT ''::text NOT NULL,
	oauth_access_token text DEFAULT ''::text NOT NULL,
	oauth_refresh_token text DEFAULT ''::text NOT NULL,
	oauth_expiry timestamp with time zone DEFAULT '0001-01-01 00:00:00+00'::timestamp with time zone NOT NULL,
	PRIMARY KEY(user_id, login_type),
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- This migrates columns on api_keys to the new user_links table.
-- It does this by finding all the API keys for each user, choosing
-- the most recently updated for each one and then assigning its relevant
-- values to the user_links table.
-- A user should at most have a row for an OIDC account and a Github account.
-- 'password' login types are ignored.

INSERT INTO user_links
	(
		user_id,
		login_type,
		linked_id,
		oauth_access_token,
		oauth_refresh_token,
		oauth_expiry
	)
SELECT
	keys.user_id,
	keys.login_type,
	'',
	keys.oauth_access_token,
	keys.oauth_refresh_token,
	keys.oauth_expiry
FROM
	(
		SELECT
			row_number() OVER (partition by user_id, login_type ORDER BY last_used DESC) AS x,
			api_keys.* FROM api_keys
	) as keys
WHERE x=1 AND keys.login_type != 'password';

-- Drop columns that have been migrated to user_links.
-- It appears the 'oauth_id_token' was unused and so it has
-- been dropped here as well to avoid future confusion.
ALTER TABLE api_keys
	DROP COLUMN oauth_access_token,
	DROP COLUMN oauth_refresh_token,
	DROP COLUMN oauth_id_token,
	DROP COLUMN oauth_expiry;

ALTER TABLE users ADD COLUMN login_type login_type NOT NULL DEFAULT 'password';

UPDATE
	users
SET
	login_type =  (
		SELECT
			login_type
		FROM
			user_links
		WHERE
			user_links.user_id = users.id
		ORDER BY oauth_expiry DESC
		LIMIT 1
	)
FROM
	user_links
WHERE
	user_links.user_id = users.id;
