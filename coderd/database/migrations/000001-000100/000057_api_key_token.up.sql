CREATE TYPE new_logintype AS ENUM (
	'password',
	'github',
	'oidc',
	'token'
);

ALTER TABLE users
	ALTER COLUMN login_type DROP DEFAULT,
	ALTER COLUMN login_type TYPE new_logintype USING (login_type::text::new_logintype),
	ALTER COLUMN login_type SET DEFAULT 'password'::new_logintype;

ALTER TABLE user_links
	ALTER COLUMN login_type TYPE new_logintype USING (login_type::text::new_logintype);

ALTER TABLE api_keys
	ALTER COLUMN login_type TYPE new_logintype USING (login_type::text::new_logintype);

DROP TYPE login_type;
ALTER TYPE new_logintype RENAME TO login_type;
