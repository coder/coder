CREATE UNIQUE INDEX IF NOT EXISTS users_email_lower_idx ON users USING btree (lower(email)) WHERE (deleted = false);
