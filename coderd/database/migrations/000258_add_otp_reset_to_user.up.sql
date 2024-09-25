ALTER TABLE users ADD COLUMN hashed_one_time_passcode bytea;
COMMENT ON COLUMN users.hashed_one_time_passcode IS 'A hash of the one-time-passcode given to the user.';

ALTER TABLE users ADD COLUMN one_time_passcode_expires_at timestamp with time zone;
COMMENT ON COLUMN users.one_time_passcode_expires_at IS 'The time when the one-time-passcode expires.';

ALTER TABLE users ADD COLUMN must_reset_password bool NOT NULL DEFAULT false;
COMMENT ON COLUMN users.must_reset_password IS 'Determines if the user should be forced to change their password.';
