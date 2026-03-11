ALTER TABLE users ADD COLUMN is_service_account boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN users.is_service_account IS 'Determines if a user is an admin-managed account that cannot login';

-- Service accounts must use login_type 'none'.
ALTER TABLE users ADD CONSTRAINT users_service_account_login_type CHECK (is_service_account = false OR login_type = 'none');

-- Paranoia check: mark any (unlikely) existing user with an empty email as a
-- service account so that adding the constraint below does not fail.
-- NOTE: considered setting email to nobody@localhost instead but for all we
-- know it may already exist, so chose the lesser of two evils.
UPDATE users SET is_service_account = true, login_type = 'none' WHERE email = '';

-- Service accounts must have empty email; other users must not.
ALTER TABLE users ADD CONSTRAINT users_email_not_empty CHECK ((is_service_account = true) = (email = ''));

-- Exclude empty emails from uniqueness so multiple service accounts can omit an
-- email without conflicting. This is the less invasive alternative to making
-- email nullable, which would require a big refactor.
DROP INDEX idx_users_email;
DROP INDEX users_email_lower_idx;
CREATE UNIQUE INDEX idx_users_email ON users USING btree (email) WHERE (deleted = false AND email != '');
CREATE UNIQUE INDEX users_email_lower_idx ON users USING btree (lower(email)) WHERE (deleted = false AND email != '');
