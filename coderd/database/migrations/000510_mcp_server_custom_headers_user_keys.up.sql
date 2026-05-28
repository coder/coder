ALTER TABLE mcp_server_configs
    ADD COLUMN custom_headers_user_keys TEXT[] NOT NULL DEFAULT '{}';

CREATE TABLE mcp_server_user_header_values (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mcp_server_config_id UUID NOT NULL REFERENCES mcp_server_configs(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- JSON object {header: value} of values supplied by the user for the
    -- headers listed in mcp_server_configs.custom_headers_user_keys. Stored
    -- encrypted at rest via dbcrypt (the key id is header_values_key_id).
    header_values TEXT NOT NULL DEFAULT '{}',
    header_values_key_id TEXT REFERENCES dbcrypt_keys(active_key_digest),

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (mcp_server_config_id, user_id)
);

CREATE INDEX idx_mcp_server_user_header_values_user_id
    ON mcp_server_user_header_values(user_id);
