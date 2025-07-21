-- Stores encrypted user secrets (global, available across all organizations)
CREATE TABLE user_secrets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL,

    -- The encrypted secret value (base64-encoded encrypted data)
    value TEXT NOT NULL,

    -- The ID of the key used to encrypt the secret value.
    -- If this is NULL, the secret value is not encrypted.
    value_key_id TEXT REFERENCES dbcrypt_keys(active_key_digest),

	-- Auto-injection settings
	-- Environment variable name (e.g., "DATABASE_PASSWORD", "API_KEY")
	-- Empty string means don't inject as env var
	env_name TEXT NOT NULL DEFAULT '',

	-- File path where secret should be written (e.g., "/home/coder/.ssh/id_rsa")
	-- Empty string means don't inject as file
	file_path TEXT NOT NULL DEFAULT '',

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- Unique constraint: user can't have duplicate secret names
CREATE UNIQUE INDEX user_secrets_user_name_idx ON user_secrets(user_id, name);
