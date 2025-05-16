CREATE TABLE IF NOT EXISTS chats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    title TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS chat_messages (
    -- BIGSERIAL is auto-incrementing so we know the exact order of messages.
    id BIGSERIAL PRIMARY KEY,
    chat_id UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    model TEXT NOT NULL,
    provider TEXT NOT NULL,
    content JSONB NOT NULL
);
