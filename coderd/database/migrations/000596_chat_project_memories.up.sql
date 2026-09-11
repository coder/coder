ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'chat_project_memory';

ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_project_memory:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_project_memory:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_project_memory:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_project_memory:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_project_memory:delete';

CREATE TABLE chat_project_memories (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES chat_projects(id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL,
    body text NOT NULL,
    source_chat_id uuid REFERENCES chats(id) ON DELETE SET NULL,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chat_project_memories_name_format CHECK (name ~ '^[a-z0-9][a-z0-9_-]{0,63}$'),
    CONSTRAINT chat_project_memories_description_length CHECK (length(description) <= 150),
    CONSTRAINT chat_project_memories_body_length CHECK (octet_length(body) <= 8192)
);

COMMENT ON TABLE chat_project_memories IS 'Organization-scoped durable memories for chat projects.';

CREATE UNIQUE INDEX idx_chat_project_memories_project_lower_name ON chat_project_memories (project_id, lower(name));
CREATE INDEX idx_chat_project_memories_project_updated_at ON chat_project_memories (project_id, updated_at DESC);

CREATE TABLE chat_project_memory_cursors (
    chat_id uuid PRIMARY KEY REFERENCES chats(id) ON DELETE CASCADE,
    history_version bigint NOT NULL,
    extracted_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE chat_project_memory_cursors IS 'Per-chat cursors for project memory extraction.';
