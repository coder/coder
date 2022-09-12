ALTER TABLE users
    ADD COLUMN deleted boolean DEFAULT false NOT NULL;

DROP INDEX idx_users_email;
DROP INDEX idx_users_username;
CREATE UNIQUE INDEX idx_users_email ON users USING btree (email) WHERE deleted = false;
CREATE UNIQUE INDEX idx_users_username ON users USING btree (username) WHERE deleted = false;
