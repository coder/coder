ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'chat_user_memory';

ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_user_memory:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_user_memory:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_user_memory:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_user_memory:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_user_memory:delete';

CREATE TABLE chat_user_memories (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL,
    body text NOT NULL,
    source_chat_id uuid REFERENCES chats(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chat_user_memories_name_format CHECK (name ~ '^[a-z0-9][a-z0-9_-]{0,63}$'),
    CONSTRAINT chat_user_memories_description_length CHECK (length(description) <= 150),
    CONSTRAINT chat_user_memories_body_length CHECK (octet_length(body) <= 8192)
);

COMMENT ON TABLE chat_user_memories IS 'User-scoped durable memories for chat.';

CREATE UNIQUE INDEX idx_chat_user_memories_user_organization_lower_name ON chat_user_memories (user_id, organization_id, lower(name));
CREATE INDEX idx_chat_user_memories_user_organization_updated_at ON chat_user_memories (user_id, organization_id, updated_at DESC);

ALTER TABLE chat_project_memory_cursors RENAME TO chat_memory_cursors;
COMMENT ON TABLE chat_memory_cursors IS 'Per-chat cursors for memory extraction.';
