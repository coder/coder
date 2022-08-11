BEGIN;

CREATE TABLE IF NOT EXISTS users (
	user_id uuid NOT NULL,
	login_type login_type NOT NULL,
	linked_id text NOT NULL DEFAULT ''::text NOT NULL,
    oauth_access_token text DEFAULT ''::text NOT NULL,
    oauth_refresh_token text DEFAULT ''::text NOT NULL,
    oauth_id_token text DEFAULT ''::text NOT NULL,
    oauth_expiry timestamp with time zone DEFAULT '0001-01-01 00:00:00+00'::timestamp with time zone NOT NULL,
	UNIQUE(user_id, login_type),
)
ALTER TABLE users ADD COLUMN linked_id text NOT NULL DEFAULT ''; 

UPDATE 
  users
SET
  login_type = (
    SELECT 
      login_type
    FROM
      api_keys
    WHERE
      api_keys.user_id = users.id
    ORDER BY updated_at DESC
    LIMIT 1
  );

COMMIT;
