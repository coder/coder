ALTER TABLE users
	ADD COLUMN is_system bool DEFAULT false;

CREATE INDEX user_is_system_idx ON users USING btree (is_system);

COMMENT ON COLUMN users.is_system IS 'Determines if a user is a system user, and therefore cannot login or perform normal actions';
