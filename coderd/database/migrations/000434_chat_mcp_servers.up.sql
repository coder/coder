CREATE TABLE chat_mcp_servers (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug                 TEXT NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9_-]*$'),
    url                  TEXT NOT NULL,
    display_name         TEXT NOT NULL DEFAULT '',
    auth_type            TEXT NOT NULL DEFAULT 'none' CHECK (auth_type IN ('none', 'header', 'oauth')),
    auth_headers         TEXT NOT NULL DEFAULT '',
    auth_headers_key_id  TEXT REFERENCES dbcrypt_keys(active_key_digest),
    oauth_client_id      TEXT NOT NULL DEFAULT '',
    oauth_auth_server    TEXT NOT NULL DEFAULT '',
    tool_allow_regex     TEXT NOT NULL DEFAULT '',
    tool_deny_regex      TEXT NOT NULL DEFAULT '',
    enabled              BOOLEAN NOT NULL DEFAULT TRUE,
    created_by           UUID REFERENCES users(id),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON COLUMN chat_mcp_servers.auth_headers_key_id IS 'The ID of the key used to encrypt the auth headers. If this is NULL, the headers are not encrypted';

CREATE INDEX idx_chat_mcp_servers_enabled ON chat_mcp_servers(enabled);
