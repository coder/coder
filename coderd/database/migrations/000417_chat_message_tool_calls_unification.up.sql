-- This migration intentionally recreates chat_messages in the new shape.
DROP TABLE IF EXISTS chat_messages;

CREATE TABLE chat_messages (
    id              BIGSERIAL   PRIMARY KEY,
    chat_id         UUID        NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    role            TEXT        NOT NULL, -- 'user', 'assistant', 'system', 'tool'
    content         JSONB,                -- Text content or structured data
    tool_call_id    TEXT,                 -- For tool result messages
    thinking        TEXT,                 -- Extended thinking content (if any)
    hidden          BOOLEAN     NOT NULL DEFAULT FALSE -- For system/hidden messages
);

CREATE INDEX idx_chat_messages_chat ON chat_messages(chat_id);
CREATE INDEX idx_chat_messages_chat_created ON chat_messages(chat_id, created_at);
