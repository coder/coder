CREATE TABLE chat_git_sync_rate_limits (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    host text NOT NULL,
    retry_after timestamptz NOT NULL,
    PRIMARY KEY (user_id, host)
);

COMMENT ON TABLE chat_git_sync_rate_limits IS 'Provider cooldowns shared by gitsync workers for each user and origin host.';
