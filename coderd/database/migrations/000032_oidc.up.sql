CREATE TYPE new_login_type AS ENUM (
    'password',
    'github',
    'oidc'
);
ALTER TABLE api_keys ALTER COLUMN login_type TYPE new_login_type USING (login_type::text::new_login_type);
DROP TYPE login_type;
ALTER TYPE new_login_type RENAME TO login_type;
