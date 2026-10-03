CREATE TYPE chat_status AS ENUM (
    'waiting',
    'pending',
    'running',
    'paused',
    'completed',
    'error'
);

CREATE TYPE chat_message_visibility AS ENUM (
    'user',
    'model',
    'both'
);

CREATE TABLE chats (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id            UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    workspace_id        UUID        REFERENCES workspaces(id) ON DELETE SET NULL,
    workspace_agent_id  UUID        REFERENCES workspace_agents(id) ON DELETE SET NULL,
    title               TEXT        NOT NULL DEFAULT 'New Chat',
    status              chat_status NOT NULL DEFAULT 'waiting',
    worker_id           UUID,
    started_at          TIMESTAMPTZ,
    heartbeat_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    parent_chat_id      UUID        REFERENCES chats(id) ON DELETE SET NULL,
    root_chat_id        UUID        REFERENCES chats(id) ON DELETE SET NULL,
    last_model_config_id UUID        NOT NULL
);

CREATE INDEX idx_chats_owner ON chats(owner_id);
CREATE INDEX idx_chats_workspace ON chats(workspace_id);
CREATE INDEX idx_chats_pending ON chats(status) WHERE status = 'pending';
CREATE INDEX idx_chats_parent_chat_id ON chats(parent_chat_id);
CREATE INDEX idx_chats_root_chat_id ON chats(root_chat_id);
CREATE INDEX idx_chats_last_model_config_id ON chats(last_model_config_id);

CREATE TABLE chat_messages (
    id                      BIGSERIAL   PRIMARY KEY,
    chat_id                 UUID        NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    model_config_id         UUID,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    role                    TEXT        NOT NULL,
    content                 JSONB,
    visibility              chat_message_visibility NOT NULL DEFAULT 'both',
    input_tokens            BIGINT,
    output_tokens           BIGINT,
    total_tokens            BIGINT,
    reasoning_tokens        BIGINT,
    cache_creation_tokens   BIGINT,
    cache_read_tokens       BIGINT,
    context_limit           BIGINT,
    compressed              BOOLEAN     NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_chat_messages_chat ON chat_messages(chat_id);
CREATE INDEX idx_chat_messages_chat_created ON chat_messages(chat_id, created_at);
CREATE INDEX idx_chat_messages_compressed_summary_boundary
    ON chat_messages(chat_id, created_at DESC, id DESC)
    WHERE compressed = TRUE
        AND role = 'system'
        AND visibility IN ('model', 'both');

CREATE TABLE chat_diff_statuses (
    chat_id             UUID        PRIMARY KEY REFERENCES chats(id) ON DELETE CASCADE,
    url                 TEXT,
    pull_request_state  TEXT,
    changes_requested   BOOLEAN     NOT NULL DEFAULT FALSE,
    additions           INTEGER     NOT NULL DEFAULT 0,
    deletions           INTEGER     NOT NULL DEFAULT 0,
    changed_files       INTEGER     NOT NULL DEFAULT 0,
    refreshed_at        TIMESTAMPTZ,
    stale_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    git_branch          TEXT        NOT NULL DEFAULT '',
    git_remote_origin   TEXT        NOT NULL DEFAULT ''
);

CREATE INDEX idx_chat_diff_statuses_stale_at ON chat_diff_statuses(stale_at);

CREATE TABLE chat_providers (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    provider        TEXT        NOT NULL UNIQUE,
    display_name    TEXT        NOT NULL DEFAULT '',
    api_key         TEXT        NOT NULL DEFAULT '',
    api_key_key_id  TEXT        REFERENCES dbcrypt_keys(active_key_digest),
    created_by      UUID        REFERENCES users(id),
    enabled         BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    base_url        TEXT        NOT NULL DEFAULT '',
    CONSTRAINT chat_providers_provider_check CHECK (
        provider = ANY (
            ARRAY[
                'anthropic'::text,
                'azure'::text,
                'bedrock'::text,
                'google'::text,
                'openai'::text,
                'openai-compat'::text,
                'openrouter'::text,
                'vercel'::text
            ]
        )
    )
);

COMMENT ON COLUMN chat_providers.api_key_key_id IS 'The ID of the key used to encrypt the provider API key. If this is NULL, the API key is not encrypted';

CREATE INDEX idx_chat_providers_enabled ON chat_providers(enabled);

CREATE TABLE chat_model_configs (
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    provider                TEXT        NOT NULL REFERENCES chat_providers(provider) ON DELETE CASCADE,
    model                   TEXT        NOT NULL,
    display_name            TEXT        NOT NULL DEFAULT '',
    created_by              UUID        REFERENCES users(id),
    updated_by              UUID        REFERENCES users(id),
    enabled                 BOOLEAN     NOT NULL DEFAULT TRUE,
    is_default              BOOLEAN     NOT NULL DEFAULT FALSE,
    deleted                 BOOLEAN     NOT NULL DEFAULT FALSE,
    deleted_at              TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    context_limit           BIGINT      NOT NULL,
    compression_threshold   INTEGER     NOT NULL,
    options                 JSONB       NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT chat_model_configs_context_limit_check
        CHECK (context_limit > 0),
    CONSTRAINT chat_model_configs_compression_threshold_check
        CHECK (compression_threshold >= 0 AND compression_threshold <= 100)
);

CREATE INDEX idx_chat_model_configs_enabled ON chat_model_configs(enabled);
CREATE INDEX idx_chat_model_configs_provider ON chat_model_configs(provider);
CREATE INDEX idx_chat_model_configs_provider_model
    ON chat_model_configs(provider, model);
CREATE UNIQUE INDEX idx_chat_model_configs_single_default
    ON chat_model_configs ((1))
    WHERE is_default = TRUE
        AND deleted = FALSE;

ALTER TABLE chat_messages
    ADD CONSTRAINT chat_messages_model_config_id_fkey
    FOREIGN KEY (model_config_id) REFERENCES chat_model_configs(id);

ALTER TABLE chats
    ADD CONSTRAINT chats_last_model_config_id_fkey
    FOREIGN KEY (last_model_config_id) REFERENCES chat_model_configs(id);

CREATE TABLE chat_queued_messages (
    id          BIGSERIAL   PRIMARY KEY,
    chat_id     UUID        NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    content     JSONB       NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chat_queued_messages_chat_id ON chat_queued_messages(chat_id);

ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat:*';
