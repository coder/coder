ALTER TABLE users ADD COLUMN git_sync_retry_after timestamptz;

COMMENT ON COLUMN users.git_sync_retry_after IS 'The earliest time gitsync may refresh this user after a provider rate limit.';
