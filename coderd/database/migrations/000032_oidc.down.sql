CREATE TYPE old_login_type AS ENUM (
    'password',
    'github'
);
ALTER TABLE api_keys ALTER COLUMN login_type TYPE old_login_type USING (login_type::text::old_login_type);
DROP TYPE login_type;
ALTER TYPE old_login_type RENAME TO login_type;
