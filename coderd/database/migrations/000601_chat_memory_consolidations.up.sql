CREATE TYPE chat_memory_consolidation_status AS ENUM (
    'running',
    'succeeded',
    'failed',
    'skipped'
);

CREATE TABLE chat_memory_consolidations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id uuid REFERENCES chat_projects(id) ON DELETE CASCADE,
    user_id uuid REFERENCES users(id) ON DELETE CASCADE,
    status chat_memory_consolidation_status NOT NULL,
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    model text NOT NULL DEFAULT '',
    memories_before int NOT NULL DEFAULT 0,
    memories_after int NOT NULL DEFAULT 0,
    mutations jsonb NOT NULL DEFAULT '[]',
    error text NOT NULL DEFAULT '',
    CONSTRAINT chat_memory_consolidations_scope CHECK ((project_id IS NULL) <> (user_id IS NULL))
);

COMMENT ON TABLE chat_memory_consolidations IS 'Bounded journal of detached chat-memory consolidation runs.';

CREATE INDEX idx_chat_memory_consolidations_project_started_at
    ON chat_memory_consolidations (project_id, started_at DESC);
CREATE INDEX idx_chat_memory_consolidations_user_organization_started_at
    ON chat_memory_consolidations (user_id, organization_id, started_at DESC);
