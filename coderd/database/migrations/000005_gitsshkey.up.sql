CREATE TABLE IF NOT EXISTS gitsshkeys (
    user_id uuid PRIMARY KEY NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    private_key text NOT NULL,
    public_key text NOT NULL
);
