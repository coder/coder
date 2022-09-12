ALTER TABLE users
    ADD COLUMN deleted boolean DEFAULT false NOT NULL;

DROP INDEX idx_users_email;
DROP INDEX idx_users_username;
DROP INDEX users_username_lower_idx;
CREATE UNIQUE INDEX idx_users_email ON users USING btree (email) WHERE deleted = false;
CREATE UNIQUE INDEX idx_users_username ON users USING btree (username) WHERE deleted = false;
CREATE UNIQUE INDEX users_username_lower_idx ON users USING btree (lower(username)) WHERE deleted = false;
