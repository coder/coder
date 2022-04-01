CREATE TABLE IF NOT EXISTS git_ssh_keys (
    user_id text PRIMARY KEY NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    private_key bytea NOT NULL,
    public_key bytea NOT NULL
);
