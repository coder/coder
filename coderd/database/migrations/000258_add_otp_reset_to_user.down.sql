ALTER TABLE users DROP CONSTRAINT one_time_passcode_set;

ALTER TABLE users DROP COLUMN hashed_one_time_passcode;
ALTER TABLE users DROP COLUMN one_time_passcode_expires_at;
ALTER TABLE users DROP COLUMN must_reset_password;
