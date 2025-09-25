-- Drop all CHECK constraints added in the up migration
ALTER TABLE api_keys
DROP CONSTRAINT api_keys_lifetime_seconds_positive;

ALTER TABLE api_keys
DROP CONSTRAINT api_keys_expires_at_after_created;
