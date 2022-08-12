BEGIN;

CREATE TABLE IF NOT EXISTS user_links (
	user_id uuid NOT NULL,
	login_type login_type NOT NULL,
	linked_id text DEFAULT ''::text NOT NULL,
	oauth_access_token text DEFAULT ''::text NOT NULL,
	oauth_refresh_token text DEFAULT ''::text NOT NULL,
	oauth_expiry timestamp with time zone DEFAULT '0001-01-01 00:00:00+00'::timestamp with time zone NOT NULL,
	UNIQUE(user_id, login_type)
);

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
			row_number() OVER (partition by user_id, login_type ORDER BY updated_at DESC) AS x, 
			api_keys.* FROM api_keys
	) as keys
 WHERE x=1 AND keys.login_type != 'password';

ALTER TABLE api_keys 
	DROP COLUMN oauth_access_token,
	DROP COLUMN oauth_refresh_token,
	DROP COLUMN oauth_id_token,
	DROP COLUMN oauth_expiry;

COMMIT;
