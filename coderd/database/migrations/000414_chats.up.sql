CREATE TYPE chat_status AS ENUM (
    'waiting',    -- Waiting for user input or workspace
    'pending',    -- Queued, waiting for a coderd replica to pick up
    'running',    -- Being processed by a coderd replica
    'paused',     -- Manually paused by user
    'completed',  -- Finished (no pending work)
    'error'       -- Failed, needs user intervention
);

CREATE TABLE chats (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id            UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    workspace_id        UUID        REFERENCES workspaces(id) ON DELETE SET NULL,
    workspace_agent_id  UUID        REFERENCES workspace_agents(id) ON DELETE SET NULL,
    title               TEXT        NOT NULL DEFAULT 'New Chat',
    status              chat_status NOT NULL DEFAULT 'waiting',
    model_config        JSONB       NOT NULL DEFAULT '{}',
    -- Locking fields for multi-replica safety
    worker_id           UUID,
    started_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chats_owner ON chats(owner_id);
CREATE INDEX idx_chats_workspace ON chats(workspace_id);
CREATE INDEX idx_chats_pending ON chats(status) WHERE status = 'pending';

CREATE TABLE chat_messages (
    id              BIGSERIAL   PRIMARY KEY,
    chat_id         UUID        NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    role            TEXT        NOT NULL, -- 'user', 'assistant', 'system', 'tool'
    content         JSONB,                -- Text content or structured data
    tool_calls      JSONB,                -- For assistant messages with tool calls
    tool_call_id    TEXT,                 -- For tool result messages
    thinking        TEXT,                 -- Extended thinking content (if any)
    hidden          BOOLEAN     NOT NULL DEFAULT FALSE -- For system/hidden messages
);

CREATE INDEX idx_chat_messages_chat ON chat_messages(chat_id);
CREATE INDEX idx_chat_messages_chat_created ON chat_messages(chat_id, created_at);
