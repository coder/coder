-- Since we can't simply delete a user that potentially has all kinds of tables
-- referencing it, give service accounts with empty emails a unique placeholder
-- so the original unique indexes can be restored. We only run down migrations
-- in dev, so hopefully this is not a big deal.
UPDATE users SET
    email = 'ex-service-account-' || id::text || '@localhost',
    is_service_account = false
WHERE is_service_account = true AND email = '';

-- Restore original unique indexes.
DROP INDEX IF EXISTS idx_users_email;
DROP INDEX IF EXISTS users_email_lower_idx;
CREATE UNIQUE INDEX idx_users_email ON users USING btree (email) WHERE (deleted = false);
CREATE UNIQUE INDEX users_email_lower_idx ON users USING btree (lower(email)) WHERE (deleted = false);

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_not_empty;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_service_account_login_type;
ALTER TABLE users DROP COLUMN is_service_account;
