-- This migration has been modified after its initial commit.
-- The new implementation makes the same changes as the original, but
-- takes into account the message in create_migration.sh. This is done
-- to allow the insertion of a user with the "none" login type in later migrations.

CREATE TYPE new_logintype AS ENUM (
  'password',
  'github',
  'oidc',
  'token',
  'none'
);
COMMENT ON TYPE new_logintype IS 'Specifies the method of authentication. "none" is a special case in which no authentication method is allowed.';

ALTER TABLE users
	ALTER COLUMN login_type DROP DEFAULT,
	ALTER COLUMN login_type TYPE new_logintype USING (login_type::text::new_logintype),
	ALTER COLUMN login_type SET DEFAULT 'password'::new_logintype;

DROP INDEX IF EXISTS idx_api_key_name;
ALTER TABLE api_keys
	ALTER COLUMN login_type TYPE new_logintype USING (login_type::text::new_logintype);
CREATE UNIQUE INDEX idx_api_key_name
ON api_keys (user_id, token_name)
WHERE (login_type = 'token'::new_logintype);

ALTER TABLE user_links
	ALTER COLUMN login_type TYPE new_logintype USING (login_type::text::new_logintype);

DROP TYPE login_type;
ALTER TYPE new_logintype RENAME TO login_type;
