BEGIN;

ALTER TABLE users ADD COLUMN login_type login_type NOT NULL DEFAULT 'password'; 
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
  )
COMMIT;
