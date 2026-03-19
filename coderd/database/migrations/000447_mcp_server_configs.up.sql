CREATE TABLE mcp_server_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Display
    display_name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    icon_url TEXT NOT NULL DEFAULT '',

    -- Connection
    transport TEXT NOT NULL DEFAULT 'streamable_http'
        CHECK (transport IN ('streamable_http', 'sse')),
    url TEXT NOT NULL,

    -- Authentication
    auth_type TEXT NOT NULL DEFAULT 'none'
        CHECK (auth_type IN ('none', 'oauth2', 'api_key', 'custom_headers')),

    -- OAuth2 config (when auth_type = 'oauth2')
    oauth2_client_id TEXT NOT NULL DEFAULT '',
    oauth2_client_secret TEXT NOT NULL DEFAULT '',
    oauth2_client_secret_key_id TEXT REFERENCES dbcrypt_keys(active_key_digest),
    oauth2_auth_url TEXT NOT NULL DEFAULT '',
    oauth2_token_url TEXT NOT NULL DEFAULT '',
    oauth2_scopes TEXT NOT NULL DEFAULT '',

    -- API key config (when auth_type = 'api_key')
    api_key_header TEXT NOT NULL DEFAULT 'Authorization',
    api_key_value TEXT NOT NULL DEFAULT '',
    api_key_value_key_id TEXT REFERENCES dbcrypt_keys(active_key_digest),

    -- Custom headers (when auth_type = 'custom_headers')
    custom_headers TEXT NOT NULL DEFAULT '{}',
    custom_headers_key_id TEXT REFERENCES dbcrypt_keys(active_key_digest),

    -- Tool governance
    tool_allow_list TEXT[] NOT NULL DEFAULT '{}',
    tool_deny_list TEXT[] NOT NULL DEFAULT '{}',

    -- Availability policy
    availability TEXT NOT NULL DEFAULT 'default_off'
        CHECK (availability IN ('force_on', 'default_on', 'default_off')),

    -- Lifecycle
    enabled BOOLEAN NOT NULL DEFAULT false,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE mcp_server_user_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mcp_server_config_id UUID NOT NULL REFERENCES mcp_server_configs(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    access_token TEXT NOT NULL,
    access_token_key_id TEXT REFERENCES dbcrypt_keys(active_key_digest),
    refresh_token TEXT NOT NULL DEFAULT '',
    refresh_token_key_id TEXT REFERENCES dbcrypt_keys(active_key_digest),
    token_type TEXT NOT NULL DEFAULT 'Bearer',
    expiry TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (mcp_server_config_id, user_id)
);

-- Add MCP server selection to chats (per-chat, like model_config_id)
ALTER TABLE chats ADD COLUMN mcp_server_ids UUID[] NOT NULL DEFAULT '{}';

CREATE INDEX idx_mcp_server_configs_enabled ON mcp_server_configs(enabled) WHERE enabled = TRUE;
CREATE INDEX idx_mcp_server_configs_forced ON mcp_server_configs(enabled, availability) WHERE enabled = TRUE AND availability = 'force_on';
CREATE INDEX idx_mcp_server_user_tokens_user_id ON mcp_server_user_tokens(user_id);
